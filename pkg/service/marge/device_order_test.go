package marge

import (
	"testing"

	"github.com/gesellix/bose-soundtouch/pkg/models"
)

func deviceNames(ds []models.AccountDevice) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return out
}

func TestSortAccountDevices(t *testing.T) {
	defer SetDeviceOrder("")

	fresh := func() []models.AccountDevice {
		return []models.AccountDevice{
			{DeviceID: "08DF1F0DC889", Name: "Front Room"},
			{DeviceID: "1893D7C5F0C7", Name: "Office"},
			{DeviceID: "2C6B7DAA77E7", Name: "Master Bedroom"},
			{DeviceID: "2C6B7DAA86C9", Name: "Kitchen"},
			{DeviceID: "2C6B7DAA8C51", Name: "Front Deck"},
		}
	}

	t.Run("empty order keeps datastore order", func(t *testing.T) {
		SetDeviceOrder("")
		ds := fresh()
		sortAccountDevices(ds)
		if got := deviceNames(ds); got[0] != "Front Room" || got[4] != "Front Deck" {
			t.Fatalf("unexpected order %v", got)
		}
	})

	t.Run("pinned names first, rest stable", func(t *testing.T) {
		SetDeviceOrder(" kitchen , Front Room,office ")
		ds := fresh()
		sortAccountDevices(ds)
		want := []string{"Kitchen", "Front Room", "Office", "Master Bedroom", "Front Deck"}
		for i, w := range want {
			if ds[i].Name != w {
				t.Fatalf("pos %d: want %q got %v", i, w, deviceNames(ds))
			}
		}
	})

	t.Run("deviceID works too and unknown entries are ignored", func(t *testing.T) {
		SetDeviceOrder("nope,2c6b7daa8c51,Office")
		ds := fresh()
		sortAccountDevices(ds)
		if ds[0].Name != "Front Deck" || ds[1].Name != "Office" || ds[2].Name != "Front Room" {
			t.Fatalf("unexpected order %v", deviceNames(ds))
		}
	})
}
