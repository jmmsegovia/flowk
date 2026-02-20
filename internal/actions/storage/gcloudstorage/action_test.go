package gcloudstorage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/mitchellh/mapstructure"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

type fakeService struct {
	objects map[string]map[string]*fakeObject
	auth    AuthInfo
	closed  bool
}

type fakeObject struct {
	name         string
	bucket       string
	data         []byte
	updated      time.Time
	contentType  string
	storageClass string
}

func newFakeService() *fakeService {
	return &fakeService{objects: make(map[string]map[string]*fakeObject)}
}

func (f *fakeService) Close() error {
	f.closed = true
	return nil
}

func (f *fakeService) ensureBucket(bucket string) {
	if _, ok := f.objects[bucket]; !ok {
		f.objects[bucket] = make(map[string]*fakeObject)
	}
}

func (f *fakeService) ObjectExists(_ context.Context, path StoragePath) (bool, error) {
	if path.Object == "" {
		_, ok := f.objects[path.Bucket]
		return ok, nil
	}
	bucket, ok := f.objects[path.Bucket]
	if !ok {
		return false, nil
	}
	_, ok = bucket[path.Object]
	return ok, nil
}

func (f *fakeService) CopyObject(_ context.Context, src, dst StoragePath) error {
	bucket, ok := f.objects[src.Bucket]
	if !ok {
		return errors.New("source bucket not found")
	}
	object, ok := bucket[src.Object]
	if !ok {
		return errors.New("source object not found")
	}
	f.ensureBucket(dst.Bucket)
	copied := *object
	copied.name = dst.Object
	f.objects[dst.Bucket][dst.Object] = &copied
	return nil
}

func (f *fakeService) DeleteObject(_ context.Context, path StoragePath) error {
	bucket, ok := f.objects[path.Bucket]
	if !ok {
		return nil
	}
	delete(bucket, path.Object)
	return nil
}

func (f *fakeService) List(_ context.Context, path StoragePath, recursive bool) (ServiceListResult, error) {
	bucket, ok := f.objects[path.Bucket]
	if !ok {
		return ServiceListResult{Exists: false}, nil
	}
	result := ServiceListResult{}
	prefix := path.Object

	if prefix == "" {
		result.Exists = true
		for name, obj := range bucket {
			result.Objects = append(result.Objects, ObjectAttrs{Name: name, Size: int64(len(obj.data)), Updated: obj.updated})
		}
		return result, nil
	}

	if !strings.HasSuffix(prefix, "/") && !recursive {
		if obj, ok := bucket[prefix]; ok {
			result.Exists = true
			result.Objects = append(result.Objects, ObjectAttrs{Name: obj.name, Size: int64(len(obj.data)), Updated: obj.updated})
		}
		return result, nil
	}

	matchPrefix := prefix
	if recursive && matchPrefix != "" && !strings.HasSuffix(matchPrefix, "/") {
		matchPrefix += "/"
	}

	for name, obj := range bucket {
		if strings.HasPrefix(name, matchPrefix) {
			result.Objects = append(result.Objects, ObjectAttrs{Name: obj.name, Size: int64(len(obj.data)), Updated: obj.updated})
		}
	}
	result.Exists = len(result.Objects) > 0
	return result, nil
}

func (f *fakeService) FetchAuthInfo(context.Context) (AuthInfo, error) {
	return f.auth, nil
}

type testLogger struct {
	logs []string
}

func (l *testLogger) Printf(format string, v ...interface{}) {
	l.logs = append(l.logs, fmt.Sprintf(format, v...))
}

func (l *testLogger) PrintColored(plain, _ string) {
	l.logs = append(l.logs, plain)
}

func TestExecuteCopySingleObject(t *testing.T) {
	ctx := context.Background()
	service := newFakeService()
	service.ensureBucket("source")
	service.objects["source"]["file.txt"] = &fakeObject{name: "file.txt", bucket: "source", data: []byte("data"), updated: time.Now()}

	logger := &testLogger{}
	execCtx := &registry.ExecutionContext{Logger: logger}

	act := action{factory: func(context.Context) (Service, error) { return service, nil }}
	payload := Payload{Operation: OperationCopy, Copy: &CopyPayload{Source: "gs://source/file.txt", Destination: "gs://target/file.txt"}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := act.Execute(ctx, raw, execCtx)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if result.Type != flow.ResultTypeJSON {
		t.Fatalf("unexpected result type: %s", result.Type)
	}

	var copyResult CopyResult
	if err := mapstructure.Decode(result.Value, &copyResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if len(copyResult.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(copyResult.Entries))
	}

	entry := copyResult.Entries[0]
	if !entry.Copied {
		t.Fatalf("expected copy to succeed: %+v", entry)
	}
	if _, ok := service.objects["target"]["file.txt"]; !ok {
		t.Fatalf("destination object missing")
	}
}

func TestExecuteMoveMissingSource(t *testing.T) {
	ctx := context.Background()
	service := newFakeService()

	act := action{factory: func(context.Context) (Service, error) { return service, nil }}
	payload := Payload{Operation: OperationMove, Move: &MovePayload{Source: "gs://bucket/missing.txt", Destination: "gs://bucket/other.txt"}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := act.Execute(ctx, raw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var moveResult MoveResult
	if err := mapstructure.Decode(result.Value, &moveResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if len(moveResult.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(moveResult.Entries))
	}
	if moveResult.Entries[0].Moved {
		t.Fatalf("expected move to be skipped: %+v", moveResult.Entries[0])
	}
}

func TestExecuteRemoveHandlesMissingTargets(t *testing.T) {
	ctx := context.Background()
	service := newFakeService()
	service.ensureBucket("data")
	service.objects["data"]["existing.txt"] = &fakeObject{name: "existing.txt", bucket: "data", data: []byte("value"), updated: time.Now()}

	act := action{factory: func(context.Context) (Service, error) { return service, nil }}
	payload := Payload{Operation: OperationRemove, Remove: &RemovePayload{Targets: []string{"gs://data/existing.txt", "gs://data/missing.txt"}}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := act.Execute(ctx, raw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var removeResult RemoveResult
	if err := mapstructure.Decode(result.Value, &removeResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if diff := cmp.Diff([]bool{true, false}, []bool{removeResult.Entries[0].Deleted, removeResult.Entries[1].Deleted}); diff != "" {
		t.Fatalf("unexpected deletion flags (-want +got):\n%s", diff)
	}
}

func TestExecuteListMissingObjectIsNotError(t *testing.T) {
	ctx := context.Background()
	service := newFakeService()

	act := action{factory: func(context.Context) (Service, error) { return service, nil }}
	payload := Payload{Operation: OperationList, List: &ListPayload{Target: "gs://bucket/missing.txt"}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := act.Execute(ctx, raw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var listResponse ListResponse
	if err := mapstructure.Decode(result.Value, &listResponse); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if listResponse.List.Exists {
		t.Fatalf("expected exists=false, got true")
	}
}

func TestExecuteAuthInfo(t *testing.T) {
	ctx := context.Background()
	service := newFakeService()
	service.auth = AuthInfo{ProjectID: "project", Account: "account@example.com", Scopes: []string{"scope"}, Source: "test"}

	act := action{factory: func(context.Context) (Service, error) { return service, nil }}
	payload := Payload{Operation: OperationAuthInfo}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	result, err := act.Execute(ctx, raw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	var authResult AuthInfoResult
	if err := mapstructure.Decode(result.Value, &authResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}

	if diff := cmp.Diff(service.auth, authResult.Info); diff != "" {
		t.Fatalf("unexpected auth info (-want +got):\n%s", diff)
	}
}
