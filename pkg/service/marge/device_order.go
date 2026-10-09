package marge

import (
	"sort"
	"strings"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

// deviceOrder holds the operator-pinned speaker order (device names or
// deviceIDs, case-insensitive). Devices listed here come first, in this order;
// everything else keeps the datastore order (ascending deviceID) after them.
//
// The SoundTouch/Stockholm app does not sort speakers itself: it rearranges
// its device array to match the order of the account devices list we return
// (discovery_controller "PARSE MARGE DEVICES" -> pushMargeOrderArr), so this is
// the one place that decides what the user sees first.
var deviceOrder []string

// SetDeviceOrder parses a comma-separated DEVICE_ORDER value. Empty disables
// pinning (pure datastore order).
func SetDeviceOrder(csv string) {
	deviceOrder = deviceOrder[:0]
	for _, part := range strings.Split(csv, ",") {
		if p := strings.ToLower(strings.TrimSpace(part)); p != "" {
			deviceOrder = append(deviceOrder, p)
		}
	}
}

func deviceRank(d models.AccountDevice) int {
	name := strings.ToLower(strings.TrimSpace(d.Name))
	id := strings.ToLower(strings.TrimSpace(d.DeviceID))
	for i, want := range deviceOrder {
		if want == name || want == id {
			return i
		}
	}
	return len(deviceOrder)
}

// sortAccountDevices applies deviceOrder; stable, so unpinned devices keep
// their incoming (deviceID) order.
func sortAccountDevices(devices []models.AccountDevice) {
	if len(deviceOrder) == 0 {
		return
	}
	sort.SliceStable(devices, func(i, j int) bool {
		return deviceRank(devices[i]) < deviceRank(devices[j])
	})
}
