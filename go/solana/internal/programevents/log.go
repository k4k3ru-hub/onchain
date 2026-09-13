package programevents

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	solana "github.com/k4k3ru-hub/onchain/go/solana"
)

type Event struct {
	Instruction string
	Data        []byte
	Index       uint32
}
type frame struct {
	program string
	events  []Event
}

// Read extracts committed data events from the specified program, preserving log positions.
//
// Returns:
//   - Events whose enclosing invocations all succeeded before any truncation marker.
//   - ErrExecutionLogsTruncated alongside the completed prefix, or a fatal error with no events.
//
// Version:
//   - 2026-09-13: Preserve committed events before truncation and validate invocation depths.
//   - 2026-09-12: Added.
//   - 2026-09-12: Describe truncated execution logs explicitly.
func Read(log *solana.Log, program solana.Address) ([]Event, error) {
	if log == nil {
		return nil, fmt.Errorf("failed to decode program events: log=null")
	}
	if log.Failed {
		return nil, nil
	}
	var stack []frame
	var result []Event
	for i, line := range log.Messages {
		if line == "Log truncated" {
			// Later short messages can survive truncation with missing invocation
			// boundaries between them. Only the completed prefix is trustworthy.
			return result, fmt.Errorf("%w: log_index=%d", solana.ErrExecutionLogsTruncated, i)
		}
		if strings.HasPrefix(line, "Program log: ") {
			if strings.HasPrefix(line, "Program log: Instruction: ") && len(stack) > 0 && stack[len(stack)-1].program == program.String() {
				stack[len(stack)-1].events = append(stack[len(stack)-1].events, Event{Instruction: strings.TrimPrefix(line, "Program log: Instruction: "), Index: uint32(i)})
			}
			continue
		}
		if strings.HasPrefix(line, "Program data: ") {
			if len(stack) > 0 && stack[len(stack)-1].program == program.String() {
				data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "Program data: "))
				if err != nil {
					return nil, fmt.Errorf("failed to decode program event data: %w", err)
				}
				stack[len(stack)-1].events = append(stack[len(stack)-1].events, Event{Data: data, Index: uint32(i)})
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "Program" && fields[2] == "invoke" {
			if len(fields) != 4 || fields[3] != "["+strconv.Itoa(len(stack)+1)+"]" {
				return nil, fmt.Errorf("failed to decode program events: invocation_depth=invalid log_index=%d", i)
			}
			stack = append(stack, frame{program: fields[1]})
			continue
		}
		if len(fields) >= 3 && fields[0] == "Program" && (fields[2] == "success" || fields[2] == "failed:") {
			if len(stack) == 0 || stack[len(stack)-1].program != fields[1] {
				return nil, fmt.Errorf("failed to decode program events: invocation_stack=invalid")
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if fields[2] == "success" {
				if len(stack) == 0 {
					result = append(result, f.events...)
				} else {
					stack[len(stack)-1].events = append(stack[len(stack)-1].events, f.events...)
				}
			}
			continue
		}
	}
	if len(stack) != 0 {
		return nil, fmt.Errorf("failed to decode program events: invocation_stack=too_short")
	}
	return result, nil
}
