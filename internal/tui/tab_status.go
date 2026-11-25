// SPDX-License-Identifier: GPL-2.0-only

package tui

import (
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/rm4n0s/tube/internal/app"
)

// statusLabelWidth is the column of the labels of the Status tab.
const statusLabelWidth = 14

// eventTime is the format of an event's time.
const eventTime = "15:04:05"

// statusBody is the Status tab (docs/SPEC.md §13.8): the application, the VPN, the network,
// the traffic, and the events that fit, the newest last.
func (m Model) statusBody() []line {
	st := m.run.state
	sty := m.styles
	width := max(m.width-statusLabelWidth, 1)
	var lines []line
	add := func(label string, style lipgloss.Style, value string) {
		lines = append(lines, text(sty.label.Render(fit(label, statusLabelWidth))+style.Render(fit(value, width))))
	}
	plain := lipgloss.NewStyle()
	application, appStyle := st.AppName+", running", sty.ok
	if m.run.ended() {
		application, appStyle = st.AppName+", ended with status "+strconv.Itoa(st.Exit), sty.warn
	}
	add("Application", appStyle, application)
	vpn, vpnStyle := vpnText(st), m.stateStyle(st.State)
	if m.run.ended() {
		vpn, vpnStyle = "closed when the application ended", sty.dim
		if st.ProfileName != "" {
			vpn = st.ProfileName + ", " + vpn
		}
	}
	add("VPN", vpnStyle, vpn)
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
	add("Address", sty.info, address)
	add("DNS servers", sty.info, dns)
	add("MTU", plain, mtu)
	lines = append(lines, line{})
	down, up := countText(st.Down.Packets), countText(st.Up.Packets)
	countWidth := max(len(down), len(up))
	traffic := func(t app.Traffic, count string) string {
		return fit(bytesText(t.Bytes), 9) + strings.Repeat(" ", countWidth-len(count)) + count + " packets   " + rateText(t.Rate)
	}
	add("↓ Received", sty.down, traffic(st.Down, down))
	add("↑ Sent", sty.up, traffic(st.Up, up))
	dropped, droppedStyle := countText(st.Dropped)+" packets", plain
	if st.Dropped == 1 {
		dropped = "1 packet"
	}
	if st.Dropped > 0 {
		droppedStyle = sty.warn
	}
	add("  Dropped", droppedStyle, dropped)
	room := m.runPageHeight() - len(lines) - 1
	if room <= 0 || len(st.Events) == 0 {
		return lines
	}
	lines = append(lines, line{})
	events := st.Events[max(len(st.Events)-room, 0):]
	for _, e := range events {
		lines = append(lines, text(sty.dim.Render(e.Time.Format(eventTime))+"  "+fit(clean(e.Text), max(m.width-len(eventTime)-2, 1))))
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
