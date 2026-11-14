package capture

import "github.com/Dardrai/airpods-gui/core/dongle/sim"

// AddSimReplies gives a simulated dongle the diagnostic records a capture reads: valid, and
// empty of anything but their layout.
func AddSimReplies(cfg *sim.Config) {
	if cfg.Replies == nil {
		cfg.Replies = map[byte]sim.Hex{}
	}
	trace := make(sim.Hex, 512)
	trace[0], trace[1] = 1, 0x10
	inspect := make(sim.Hex, 400)
	inspect[0], inspect[1], inspect[2], inspect[3] = 1, 3, 2, 1
	phone := make(sim.Hex, 52)
	phone[0], phone[3] = 1, 12
	radio := make(sim.Hex, 128)
	radio[0], radio[1], radio[64] = 1, 128, 1
	cfg.Replies[11], cfg.Replies[13], cfg.Replies[15], cfg.Replies[19] = trace, inspect, phone, radio
}
