package layer

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nspcc-dev/neo-go/pkg/crypto/keys"
	"github.com/nspcc-dev/neofs-api-go/pkg/acl/eacl"
	"github.com/nspcc-dev/neofs-api-go/pkg/container"
	cid "github.com/nspcc-dev/neofs-api-go/pkg/container/id"
	"github.com/nspcc-dev/neofs-api-go/pkg/owner"
	"github.com/nspcc-dev/neofs-s3-gw/api"
	"github.com/nspcc-dev/neofs-s3-gw/creds/accessbox"
	"github.com/nspcc-dev/neofs-sdk-go/pkg/pool"
	"go.uber.org/zap"
)

type (
	// BucketInfo stores basic bucket data.
	BucketInfo struct {
		Name    string
		CID     *cid.ID
		Owner   *owner.ID
		Created time.Time
	}

	// ListObjectsParams represents object listing request parameters.
	ListObjectsParams struct {
		Bucket    string
		Prefix    string
		Token     string
		Delimiter string
		MaxKeys   int
		Marker    string
		Version   int
	}
)

func (n *layer) containerInfo(ctx context.Context, cid *cid.ID) (*BucketInfo, error) {
	var (
		err       error
		res       *container.Container
		rid       = api.GetRequestID(ctx)
		bearerOpt = n.BearerOpt(ctx)

		info = &BucketInfo{
			CID:  cid,
			Name: cid.String(),
		}
	)
	res, err = n.pool.GetContainer(ctx, cid, bearerOpt)
	if err != nil {
		n.log.Error("could not fetch container",
			zap.Stringer("cid", cid),
			zap.String("request_id", rid),
			zap.Error(err))

		if strings.Contains(err.Error(), "container not found") {
			return nil, ErrBucketNotFound
		}
		return nil, err
	}

	info.Owner = res.OwnerID()

	for _, attr := range res.Attributes() {
		switch key, val := attr.Key(), attr.Value(); key {
		case container.AttributeName:
			info.Name = val
		case container.AttributeTimestamp:
			unix, err := strconv.ParseInt(attr.Value(), 10, 64)
			if err != nil {
				n.log.Error("could not parse container creation time",
					zap.Stringer("cid", cid),
					zap.String("request_id", rid),
					zap.String("created_at", val),
					zap.Error(err))

				continue
			}

			info.Created = time.Unix(unix, 0)
		}
	}

	return info, nil
}

func (n *layer) containerList(ctx context.Context) ([]*BucketInfo, error) {
	var (
		err       error
		own       = n.Owner(ctx)
		bearerOpt = n.BearerOpt(ctx)
		res       []*cid.ID
		rid       = api.GetRequestID(ctx)
	)
	res, err = n.pool.ListContainers(ctx, own, bearerOpt)
	if err != nil {
		n.log.Error("could not fetch container",
			zap.String("request_id", rid),
			zap.Error(err))
		return nil, err
	}

	list := make([]*BucketInfo, 0, len(res))
	for _, cid := range res {
		info, err := n.containerInfo(ctx, cid)
		if err != nil {
			n.log.Error("could not fetch container info",
				zap.String("request_id", rid),
				zap.Error(err))
			continue
		}

		list = append(list, info)
	}

	return list, nil
}

func (n *layer) createContainer(ctx context.Context, p *CreateBucketParams) (*cid.ID, error) {
	cnr := container.New(
		container.WithPolicy(p.Policy),
		container.WithCustomBasicACL(p.ACL),
		container.WithAttribute(container.AttributeName, p.Name),
		container.WithAttribute(container.AttributeTimestamp, strconv.FormatInt(time.Now().Unix(), 10)))

	var gateData *accessbox.GateData
	if data, ok := ctx.Value(api.GateData).(*accessbox.GateData); ok && data != nil {
		gateData = data
	} else {
		return nil, fmt.Errorf("couldn't get gate data from context")
	}

	cnr.SetSessionToken(gateData.SessionToken)
	cnr.SetOwnerID(n.Owner(ctx))

	cid, err := n.pool.PutContainer(ctx, cnr)
	if err != nil {
		return nil, fmt.Errorf("failed to create a bucket: %w", err)
	}

	if err = n.pool.WaitForContainerPresence(ctx, cid, pool.DefaultPollingParams()); err != nil {
		return nil, err
	}

	if err := n.setContainerEACL(ctx, cid, gateData.GateKey); err != nil {
		return nil, err
	}

	return cid, nil
}

func (n *layer) setContainerEACL(ctx context.Context, cid *cid.ID, gateKey *keys.PublicKey) error {
	if gateKey == nil {
		return fmt.Errorf("gate key must not be nil")
	}

	table := formDefaultTable(cid, *(*ecdsa.PublicKey)(gateKey))
	if err := n.pool.SetEACL(ctx, table, n.SessionOpt(ctx)); err != nil {
		return err
	}

	if err := n.waitEACLPresence(ctx, cid, defaultWaitParams()); err != nil {
		return err
	}

	return nil
}

func formDefaultTable(cid *cid.ID, gateKey ecdsa.PublicKey) *eacl.Table {
	table := eacl.NewTable()
	table.SetCID(cid)

	for op := eacl.OperationGet; op <= eacl.OperationRangeHash; op++ {
		record := eacl.NewRecord()
		record.SetOperation(op)
		record.SetAction(eacl.ActionAllow)
		eacl.AddFormedTarget(record, eacl.RoleUser, gateKey)
		table.AddRecord(record)

		record2 := eacl.NewRecord()
		record2.SetOperation(op)
		record2.SetAction(eacl.ActionDeny)
		eacl.AddFormedTarget(record2, eacl.RoleOthers)
		table.AddRecord(record2)
	}

	return table
}

type waitParams struct {
	WaitTimeout  time.Duration
	PollInterval time.Duration
}

func defaultWaitParams() *waitParams {
	return &waitParams{
		WaitTimeout:  60 * time.Second,
		PollInterval: 3 * time.Second,
	}
}

func (n *layer) waitEACLPresence(ctx context.Context, cid *cid.ID, params *waitParams) error {
	wctx, cancel := context.WithTimeout(ctx, params.WaitTimeout)
	defer cancel()
	ticker := time.NewTimer(params.PollInterval)
	defer ticker.Stop()
	wdone := wctx.Done()
	done := ctx.Done()
	for {
		select {
		case <-done:
			return ctx.Err()
		case <-wdone:
			return wctx.Err()
		case <-ticker.C:
			if _, err := n.pool.GetEACL(ctx, cid); err == nil {
				return nil
			}
			ticker.Reset(params.PollInterval)
		}
	}
}

func (n *layer) deleteContainer(ctx context.Context, cid *cid.ID) error {
	return n.pool.DeleteContainer(ctx, cid, n.SessionOpt(ctx))
}
