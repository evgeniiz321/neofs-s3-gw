package layer

import (
	"context"
	"sort"
	"strings"

	"github.com/nspcc-dev/neofs-s3-gw/api/data"
	cid "github.com/nspcc-dev/neofs-sdk-go/container/id"
	oid "github.com/nspcc-dev/neofs-sdk-go/object/id"
)

type TreeServiceMock struct {
	settings map[string]*data.BucketSettings
	versions map[string]map[string][]*data.NodeVersion
	system   map[string]map[string]*data.BaseNodeVersion
	locks    map[string]map[uint64]*data.LockInfo
}

func (t *TreeServiceMock) GetObjectTagging(ctx context.Context, cnrID *cid.ID, objVersion *data.NodeVersion) (map[string]string, error) {
	//TODO implement me
	panic("implement me")
}

func (t *TreeServiceMock) PutObjectTagging(ctx context.Context, cnrID *cid.ID, objVersion *data.NodeVersion, tagSet map[string]string) error {
	//TODO implement me
	panic("implement me")
}

func (t *TreeServiceMock) DeleteObjectTagging(ctx context.Context, cnrID *cid.ID, objVersion *data.NodeVersion) error {
	//TODO implement me
	panic("implement me")
}

func (t *TreeServiceMock) GetBucketTagging(ctx context.Context, cnrID *cid.ID) (map[string]string, error) {
	//TODO implement me
	panic("implement me")
}

func (t *TreeServiceMock) PutBucketTagging(ctx context.Context, cnrID *cid.ID, tagSet map[string]string) error {
	//TODO implement me
	panic("implement me")
}

func (t *TreeServiceMock) DeleteBucketTagging(ctx context.Context, cnrID *cid.ID) error {
	//TODO implement me
	panic("implement me")
}

func NewTreeService() *TreeServiceMock {
	return &TreeServiceMock{
		settings: make(map[string]*data.BucketSettings),
		versions: make(map[string]map[string][]*data.NodeVersion),
		system:   make(map[string]map[string]*data.BaseNodeVersion),
		locks:    make(map[string]map[uint64]*data.LockInfo),
	}
}

func (t *TreeServiceMock) PutSettingsNode(_ context.Context, id *cid.ID, settings *data.BucketSettings) error {
	t.settings[id.EncodeToString()] = settings
	return nil
}

func (t *TreeServiceMock) GetSettingsNode(_ context.Context, id *cid.ID) (*data.BucketSettings, error) {
	settings, ok := t.settings[id.EncodeToString()]
	if !ok {
		return nil, ErrNodeNotFound
	}

	return settings, nil
}

func (t *TreeServiceMock) GetNotificationConfigurationNode(ctx context.Context, cnrID *cid.ID) (*oid.ID, error) {
	panic("implement me")
}

func (t *TreeServiceMock) PutNotificationConfigurationNode(ctx context.Context, cnrID *cid.ID, objID *oid.ID) (*oid.ID, error) {
	panic("implement me")
}

func (t *TreeServiceMock) GetBucketCORS(ctx context.Context, cnrID *cid.ID) (*oid.ID, error) {
	panic("implement me")
}

func (t *TreeServiceMock) PutBucketCORS(ctx context.Context, cnrID *cid.ID, objID *oid.ID) (*oid.ID, error) {
	panic("implement me")
}

func (t *TreeServiceMock) DeleteBucketCORS(ctx context.Context, cnrID *cid.ID) (*oid.ID, error) {
	panic("implement me")
}

func (t *TreeServiceMock) GetVersions(_ context.Context, cnrID *cid.ID, objectName string) ([]*data.NodeVersion, error) {
	cnrVersionsMap, ok := t.versions[cnrID.EncodeToString()]
	if !ok {
		return nil, ErrNodeNotFound
	}

	versions, ok := cnrVersionsMap[objectName]
	if !ok {
		return nil, ErrNodeNotFound
	}

	return versions, nil
}

func (t *TreeServiceMock) GetLatestVersion(_ context.Context, cnrID *cid.ID, objectName string) (*data.NodeVersion, error) {
	cnrVersionsMap, ok := t.versions[cnrID.EncodeToString()]
	if !ok {
		return nil, ErrNodeNotFound
	}

	versions, ok := cnrVersionsMap[objectName]
	if !ok {
		return nil, ErrNodeNotFound
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].ID < versions[j].ID
	})

	if len(versions) != 0 {
		return versions[len(versions)-1], nil
	}

	return nil, ErrNodeNotFound
}

func (t *TreeServiceMock) GetLatestVersionsByPrefix(_ context.Context, cnrID *cid.ID, prefix string) ([]oid.ID, error) {
	cnrVersionsMap, ok := t.versions[cnrID.EncodeToString()]
	if !ok {
		return nil, ErrNodeNotFound
	}

	var result []oid.ID

	for key, versions := range cnrVersionsMap {
		if !strings.HasPrefix(key, prefix) {
			continue
		}

		sort.Slice(versions, func(i, j int) bool {
			return versions[i].ID < versions[j].ID
		})

		if len(versions) != 0 {
			result = append(result, versions[len(versions)-1].OID)
		}
	}

	return result, nil
}

func (t *TreeServiceMock) GetUnversioned(_ context.Context, cnrID *cid.ID, objectName string) (*data.NodeVersion, error) {
	cnrVersionsMap, ok := t.versions[cnrID.EncodeToString()]
	if !ok {
		return nil, ErrNodeNotFound
	}

	versions, ok := cnrVersionsMap[objectName]
	if !ok {
		return nil, ErrNodeNotFound
	}

	for _, version := range versions {
		if version.IsUnversioned {
			return version, nil
		}
	}

	return nil, ErrNodeNotFound
}

func (t *TreeServiceMock) AddVersion(_ context.Context, cnrID *cid.ID, objectName string, newVersion *data.NodeVersion) error {
	cnrVersionsMap, ok := t.versions[cnrID.EncodeToString()]
	if !ok {
		t.versions[cnrID.EncodeToString()] = map[string][]*data.NodeVersion{
			objectName: {newVersion},
		}
		return nil
	}

	versions, ok := cnrVersionsMap[objectName]
	if !ok {
		cnrVersionsMap[objectName] = []*data.NodeVersion{newVersion}
		return nil
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].ID < versions[j].ID
	})

	if len(versions) != 0 {
		newVersion.ID = versions[len(versions)-1].ID + 1
		newVersion.Timestamp = versions[len(versions)-1].Timestamp + 1
	}

	result := versions

	if newVersion.IsUnversioned {
		result = make([]*data.NodeVersion, 0, len(versions))
		for _, node := range versions {
			if !node.IsUnversioned {
				result = append(result, node)
			}
		}
	}

	cnrVersionsMap[objectName] = append(result, newVersion)

	return nil
}

func (t *TreeServiceMock) RemoveVersion(_ context.Context, cnrID *cid.ID, nodeID uint64) error {
	cnrVersionsMap, ok := t.versions[cnrID.EncodeToString()]
	if !ok {
		return ErrNodeNotFound
	}

	for key, versions := range cnrVersionsMap {
		for i, node := range versions {
			if node.ID == nodeID {
				cnrVersionsMap[key] = append(versions[:i], versions[i+1:]...)
				return nil
			}
		}
	}

	return ErrNodeNotFound
}

func (t *TreeServiceMock) GetAllVersionsByPrefix(_ context.Context, cnrID *cid.ID, prefix string) ([]*data.NodeVersion, error) {
	cnrVersionsMap, ok := t.versions[cnrID.EncodeToString()]
	if !ok {
		return nil, nil
	}

	var result []*data.NodeVersion
	for objName, versions := range cnrVersionsMap {
		if strings.HasPrefix(objName, prefix) {
			result = append(result, versions...)
		}
	}

	return result, nil
}

func (t *TreeServiceMock) CreateMultipartUpload(ctx context.Context, cnrID *cid.ID, info *data.MultipartInfo) error {
	panic("implement me")
}

func (t *TreeServiceMock) GetMultipartUploadsByPrefix(ctx context.Context, cnrID *cid.ID, prefix string) ([]*data.MultipartInfo, error) {
	panic("implement me")
}

func (t *TreeServiceMock) GetMultipartUpload(ctx context.Context, cnrID *cid.ID, objectName, uploadID string) (*data.MultipartInfo, error) {
	panic("implement me")
}

func (t *TreeServiceMock) AddPart(ctx context.Context, cnrID *cid.ID, multipartNodeID uint64, info *data.PartInfo) (oldObjIDToDelete *oid.ID, err error) {
	panic("implement me")
}

func (t *TreeServiceMock) GetParts(ctx context.Context, cnrID *cid.ID, multipartNodeID uint64) ([]*data.PartInfo, error) {
	panic("implement me")
}

func (t *TreeServiceMock) DeleteMultipartUpload(ctx context.Context, cnrID *cid.ID, multipartNodeID uint64) error {
	panic("implement me")
}

func (t *TreeServiceMock) PutLock(ctx context.Context, cnrID *cid.ID, nodeID uint64, lock *data.LockInfo) error {
	cnrLockMap, ok := t.locks[cnrID.EncodeToString()]
	if !ok {
		t.locks[cnrID.EncodeToString()] = map[uint64]*data.LockInfo{
			nodeID: lock,
		}
		return nil
	}

	cnrLockMap[nodeID] = lock

	return nil
}

func (t *TreeServiceMock) GetLock(ctx context.Context, cnrID *cid.ID, nodeID uint64) (*data.LockInfo, error) {
	cnrLockMap, ok := t.locks[cnrID.EncodeToString()]
	if !ok {
		return nil, nil
	}

	return cnrLockMap[nodeID], nil
}