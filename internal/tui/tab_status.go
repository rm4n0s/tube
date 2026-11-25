// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strconv"
	"strings"

	"github.com/rm4n0s/tube/internal/app"
)

// statusLabelWidth is the column of the labels of the Status tab.
const statusLabelWidth = 14

// eventTime is the format of an event's time.
const eventTime = "15:04:05"

// statusBody is the Status tab (docs/SPEC.md §13.8): the application, the VPN, the network,
// the traffic, and the events that fit, the newest last.
func (m Model) statusBody() []string {
	st := m.run.state
	width := max(m.width-statusLabelWidth, 1)
	var lines []string
	add := func(label, value string) {
		lines = append(lines, m.styles.dim.Render(fit(label, statusLabelWidth))+fit(value, width))
	}
	application := st.AppName + ", running"
	if m.run.ended() {
		application = st.AppName + ", ended with status " + strconv.Itoa(st.Exit)
	}
	add("Application", application)
	vpn := vpnText(st)
	if m.run.ended() {
		vpn = "closed when the application ended"
		if st.ProfileName != "" {
			vpn = st.ProfileName + ", " + vpn
		}
	}
	add("VPN", vpn)
	address, dns, mtu := "-", "-", "-"
	if st.State == "connected" && !m.run.ended() {
		if len(st.Addrs) > 0 {
			address = strings.Join(st.Addrs, ", ")
		}
		if len(st.DNS) > 0 {
			dns = strings.Join(st.DNS, ", ")
		}
		if st.MTU > 0 {
			mtu = strconv.Itoa(st.MTU)
		}
	}
	add("Address", address)
	add("DNS servers", dns)
	add("MTU", mtu)
	lines = append(lines, "")
	down, up := countText(st.Down.Packets), countText(st.Up.Packets)
	countWidth := max(len(down), len(up))
	traffic := func(t app.Traffic, count string) string {
		return fit(bytesText(t.Bytes), 9) + strings.Repeat(" ", countWidth-len(count)) + count + " packets   " + rateText(t.Rate)
	}
	add("Received", traffic(st.Down, down))
	add("Sent", traffic(st.Up, up))
	dropped := countText(st.Dropped) + " packets"
	if st.Dropped == 1 {
		dropped = "1 packet"
	}
	add("Dropped", dropped)
	room := m.runPageHeight() - len(lines) - 1
	if room <= 0 || len(st.Events) == 0 {
		return lines
	}
	lines = append(lines, "")
	events := st.Events[max(len(st.Events)-room, 0):]
	for _, e := range events {
		lines = append(lines, fit(e.Time.Format(eventTime)+"  "+clean(e.Text), m.width))
	}
	return lines
}

// vpnText is the VPN's line: the profile and its state.
func vpnText(st app.RunState) string {
	var state string
	switch st.State {
	case "connected":
		state = "connected for " + durationText(st.Now.Sub(st.Since))
	case "reconnecting":
		state = "connecting again… (" + durationText(st.Now.Sub(st.Since)) + ")"
	case "disconnected":
		state = "disconnected"
	case "refused":
		state = "the server refused the login"
	}
	if st.ProfileName == "" {
		return state
	}
	return st.ProfileName + ", " + state
}
