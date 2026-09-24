package main

import "fmt"

func validateAccountInstanceTimeDelta(before, after map[uint32]int64, now int64) error {
	if before == nil || after == nil {
		return fmt.Errorf("account instance-time snapshot has no keyed rows")
	}
	for instanceID, releaseTime := range before {
		current, exists := after[instanceID]
		if !exists {
			if releaseTime >= now {
				return fmt.Errorf("active account instance lock %d was removed", instanceID)
			}
			continue
		}
		if current == releaseTime {
			continue
		}
		if releaseTime < now && current > now && current <= now+3600 {
			continue
		}
		return fmt.Errorf("account instance lock %d was refreshed or modified", instanceID)
	}
	for instanceID, releaseTime := range after {
		if _, existed := before[instanceID]; existed {
			continue
		}
		if instanceID == 0 || releaseTime <= now || releaseTime > now+3600 {
			return fmt.Errorf("new account instance lock %d has invalid release time %d", instanceID, releaseTime)
		}
	}
	return nil
}
