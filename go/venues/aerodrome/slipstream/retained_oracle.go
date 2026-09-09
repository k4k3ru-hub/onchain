package slipstream

import "fmt"

type tickObservation struct {
	timestamp   uint32
	cumulative  int64
	initialized bool
}

// retainedOracle owns every slot in the active ring, including explicitly
// uninitialized slots. Missing RPC/stream payloads must never be represented as
// uninitialized slots: they invalidate the producer instead.
type retainedOracle struct {
	slots       []tickObservation
	index, next uint16
}

func wrapTickCumulative(v int64) int64 { return ((v + (1 << 55)) & ((1 << 56) - 1)) - (1 << 55) }

func (o retainedOracle) validate() error {
	if len(o.slots) == 0 || len(o.slots) > 65535 || int(o.index) >= len(o.slots) || int(o.next) < len(o.slots) || !o.slots[o.index].initialized {
		return fmt.Errorf("failed to validate retained oracle: ring=invalid")
	}
	for _, v := range o.slots {
		if v.cumulative < -(1<<55) || v.cumulative >= 1<<55 {
			return fmt.Errorf("failed to validate retained oracle: cumulative=out_of_range")
		}
	}
	return nil
}
func (o retainedOracle) clone() retainedOracle {
	o.slots = append([]tickObservation(nil), o.slots...)
	return o
}

// write applies an observation using the tick BEFORE a Swap or in-range
// Mint/Burn, with the timestamp of that event's block. Same-block writes are no-ops.
func (o *retainedOracle) write(timestamp uint32, tick int32) error {
	if o == nil {
		return fmt.Errorf("failed to update retained oracle: oracle=null")
	}
	if err := o.validate(); err != nil {
		return fmt.Errorf("failed to update retained oracle: %w", err)
	}
	if tick < -887272 || tick > 887272 {
		return fmt.Errorf("failed to update retained oracle: tick=out_of_range")
	}
	last := o.slots[o.index]
	if timestamp == last.timestamp {
		return nil
	}
	// The stream producer verifies chronological block order separately. uint32
	// subtraction here intentionally supports the EVM timestamp wrap.
	cardinality := len(o.slots)
	if int(o.next) > cardinality && int(o.index) == cardinality-1 {
		o.slots = append(o.slots, make([]tickObservation, int(o.next)-cardinality)...)
	}
	o.index = uint16((int(o.index) + 1) % len(o.slots))
	o.slots[o.index] = tickObservation{timestamp: timestamp, cumulative: wrapTickCumulative(last.cumulative + int64(tick)*int64(timestamp-last.timestamp)), initialized: true}
	return nil
}

// observe returns available=false only for an on-chain history window that is
// too short. A missing or malformed retained ring returns an error instead.
func (o retainedOracle) observe(now, secondsAgo uint32, tick int32) (int64, bool, error) {
	if err := o.validate(); err != nil {
		return 0, false, fmt.Errorf("failed to observe retained oracle: %w", err)
	}
	if tick < -887272 || tick > 887272 {
		return 0, false, fmt.Errorf("failed to observe retained oracle: tick=out_of_range")
	}
	latest := o.slots[o.index]
	age := now - latest.timestamp
	if secondsAgo <= age {
		return wrapTickCumulative(latest.cumulative + int64(tick)*int64(age-secondsAgo)), true, nil
	}
	var older tickObservation
	haveOlder := false
	// Iterate from oldest to newest, skipping only slots proven uninitialized.
	// Comparing ages relative to now handles one uint32 timestamp wrap.
	for j := 0; j < len(o.slots); j++ {
		current := o.slots[(int(o.index)+1+j)%len(o.slots)]
		if !current.initialized {
			continue
		}
		currentAge := now - current.timestamp
		if currentAge == secondsAgo {
			return current.cumulative, true, nil
		}
		if currentAge > secondsAgo {
			older = current
			haveOlder = true
			continue
		}
		if !haveOlder {
			return 0, false, nil
		}
		duration := current.timestamp - older.timestamp
		if duration == 0 {
			return 0, false, fmt.Errorf("failed to observe retained oracle: duration=empty")
		}
		elapsed := (now - secondsAgo) - older.timestamp
		// Contract interpolation divides the int56 difference before multiplying.
		rate := wrapTickCumulative(current.cumulative-older.cumulative) / int64(duration)
		return wrapTickCumulative(older.cumulative + rate*int64(elapsed)), true, nil
	}
	return 0, false, fmt.Errorf("failed to observe retained oracle: history=invalid")
}

func (o retainedOracle) fee(config dynamicFeeInputs, now uint32, tick int32) (uint32, error) {
	if err := o.validate(); err != nil {
		return 0, fmt.Errorf("failed to quote retained fee: %w", err)
	}
	config.timestamp = now
	config.tick = tick
	config.lastObservation = o.slots[o.index].timestamp
	config.cardinality = uint16(len(o.slots))
	var err error
	config.cumulativeNow, _, err = o.observe(now, 0, tick)
	if err != nil {
		return 0, err
	}
	config.cumulativePast, config.oracleAvailable, err = o.observe(now, config.secondsAgo, tick)
	if err != nil {
		return 0, err
	}
	return calculateDynamicFee(config)
}
