package sui

import (
	"context"
	"github.com/k4k3ru-hub/onchain/go/sui/internal/rpcv2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
)

// TestScopedReadDiagnosticReferences verifies failed phases and unresolved roles without retrying.
//
// Version:
//   - 2026-10-23: Added.
func TestScopedReadDiagnosticReferences(t *testing.T) {
	for _, scenario := range []string{"created", "deleted"} {
		for _, transport := range []bool{false, true} {
			response := scopedResponse(t, scenario)
			calls := 0
			reader := &scopedLedgerFake{read: func(_ context.Context, q *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
				calls++
				if transport {
					return nil, status.Error(codes.NotFound, "fixture")
				}
				result := &rpcv2.BatchGetObjectsResponse{}
				for range q.Requests {
					result.Objects = append(result.Objects, &rpcv2.GetObjectResult{Result: &rpcv2.GetObjectResult_Error{Error: status.New(codes.NotFound, "fixture").Proto()}})
				}
				return result, nil
			}}
			_, err := hydrateScopedObjects(context.Background(), reader, scopedAddress(t, "0x9"), nil, response)
			if status.Code(err) != codes.NotFound || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
			phase := "object_result"
			if transport {
				phase = "object_rpc"
			}
			// Singleton adapters preserve the server status as an RPC error.
			if !transport {
				phase = "object_rpc"
			}
			role := "output"
			if scenario == "deleted" {
				role = "input"
			}
			for _, part := range []string{"pool_id=", "stage=" + phase, "unresolved_count=1", `"role":"` + role + `"`, `"object_id":`, `"version":`, "read_elapsed_us="} {
				if !strings.Contains(err.Error(), part) {
					t.Fatalf("missing %s: %v", part, err)
				}
			}
		}
	}
}

// TestScopedBatchDiagnosticExcludesSuccess verifies candidate matching does not depend on response order.
//
// Version:
//   - 2026-10-23: Added.
func TestScopedBatchDiagnosticExcludesSuccess(t *testing.T) {
	response := scopedResponse(t, "missing_version")
	reader := &scopedLedgerFake{read: func(_ context.Context, q *rpcv2.BatchGetObjectsRequest) (*rpcv2.BatchGetObjectsResponse, error) {
		result, err := scopedReadFixture(response.Transaction.Objects.Objects, q)
		if err != nil {
			return nil, err
		}
		if len(result.Objects) != 2 {
			t.Fatalf("batch count=%d", len(result.Objects))
		}
		result.Objects[0], result.Objects[1] = result.Objects[1], result.Objects[0]
		return result, nil
	}}
	_, err := hydrateScopedObjects(context.Background(), reader, scopedAddress(t, "0x9"), nil, response)
	if status.Code(err) != codes.NotFound {
		t.Fatal(err)
	}
	for _, s := range []string{"stage=object_result", "unresolved_count=1", `"version":11`, `"role":"output"`} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("missing %s: %v", s, err)
		}
	}
	if strings.Contains(err.Error(), `"version":10`) {
		t.Fatal("successful input marked unresolved")
	}
}
