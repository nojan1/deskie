# deskie

A small always-on desk display: a Raspberry Pi 3 with a 480×320 resistive
touchscreen showing the time, weather, upcoming calendar events, Home
Assistant sensors, controls and notifications, with a night mode.

The whole device is described declaratively with a Nix flake: the OS
(NixOS), the display/touch drivers and the UI app, `deskui`, written in Go
with no GUI framework underneath. It draws straight into the Linux
framebuffer.

## Hardware

- Raspberry Pi 3
- Waveshare 4inch RPi LCD (A): ILI9486 panel on SPI (`fbtft` driver) and an
  ADS7846 resistive touch controller. It is used rotated to landscape,
  480×320.

The panel only works with Waveshare's own `waveshare35a` device tree overlay,
which is vendored in `overlays/` (see the comments in `display.nix`).

## Repository layout

```
flake.nix            Flake: the deskui package, the NixOS system, dev shell, SD image
configuration.nix    System config: users, wifi, the deskui service, its settings
display.nix          Panel/touch: firmware config.txt, SPI, vendored overlay
overlays/            waveshare35a.dtbo (vendor binary, not in RPi firmware)
app/                 deskui, the Go UI
  main.go            Flags, startup, the event/render loop
  ui.go              The screens: home, agenda, notifications
  weather.go         Weather next to the clock: drawn icons, today's high/low
  sensors.go         Read-only Home Assistant sensor tiles
  night.go           Night mode: when to show it, the night view
  config.go          JSON config loading, $VAR secret expansion
  display*.go        Display backends (framebuffer, kitty) and the Display interface
  touch_evdev.go     ADS7846 input via evdev, affine raw->pixel calibration
  calibrate.go       -calibrate: on-screen touch calibration
  widget/            Retained-mode widget toolkit
  source/hass/       Home Assistant WebSocket client
  source/calendar/   iCalendar (Google Calendar) feed reader
```

## Architecture

```
  touch (evdev) ──▶ Display.Events ──┐
                                     ▼
  Home Assistant ──▶ hass.Client ──▶ main loop ──▶ widget.Screen ──▶ dirty rect ──▶ Display.Flush ──▶ panel
  Google Calendar ─▶ calendar ─────▶ (post)        Handle / Tick      Render         RGB565, /dev/fb1
```

**One drawing path, two backends.** The UI renders into an `image.RGBA`.
A `Display` backend pushes changed regions to the screen and delivers touch
events:

- `fb`: the Pi. Converts the dirty rectangle to RGB565 in the mmapped
  framebuffer and reads touch from evdev.
- `kitty`: development on a normal machine. Shows the frame in the kitty
  terminal via its graphics protocol, with the mouse as touch. It mimics the
  panel by only updating flushed regions, so dirty-rect bugs show up locally;
  `-show-dirty` outlines each flush.

**Single UI goroutine.** Widgets are only touched from the main loop. Data
sources run on their own goroutines and hand changes over with
`post(func())`, which the loop runs between input events and frames.

**Dirty-rectangle rendering.** The display is refreshed over SPI and the Pi 3
is slow, so nothing is redrawn unless it changed. Widgets call
`Invalidate()`; each frame the Screen repaints only the union of dirty
regions, into a scratch buffer exactly that size, and flushes just that
rectangle. Layers fully hidden behind an opaque window are skipped. A test
(`TestPartialRenderMatchesFull`) checks that every partial repaint is
identical to a full redraw. On the Pi a clock tick takes ~2 ms and a
full-screen redraw ~7 ms (`cd app && go test -run XXX -bench .`).

### Widget toolkit (`app/widget`)

A small retained-mode toolkit:

- `Widget` / `Base`: bounds, visibility, invalidation; every widget embeds
  `Base`.
- Containers: `Stack` (row/column with `Fixed`/`Auto`/`Flex` sizing),
  `Panel` (absolute placement), `Window` (titled, opaque), and `List`
  (drag-to-scroll; a drag cancels the press on the item under the finger).
- Leaves: `Label`, `Paragraph` (word-wrapped), `Clock`, `Box`, `Spacer`,
  `Button` (also as a toggle) and `ListItem`.
- `Screen` holds layers (the base UI, modal overlays with a dimmed
  backdrop), routes input top-down with gesture capture (the widget that
  takes a touch-down gets the rest of the gesture), and calls `Tick` on
  widgets that change over time.
- `Theme`: shared colors and font faces (Fira Code).

### Data sources (`app/source`)

- **Home Assistant** (`hass`): WebSocket API client. Mirrors the state of
  the entities behind the home-screen buttons, sensor tiles, weather and
  night mode's away entity, toggles buttons with service calls, fetches
  forecasts (`weather.get_forecasts`, a service with a response), and
  subscribes to a custom bus event for notifications.
  Reconnects with backoff.
- **Calendar** (`calendar`): polls iCalendar feeds such as Google Calendar's
  *secret address in iCal format*, which needs no OAuth on a headless device.
  It expands recurrences (RRULE/EXDATE/RECURRENCE-ID) via `go-ical` and
  `rrule-go`, in the display's time zone. A failing feed keeps its last
  good events.

## Configuration and secrets

`deskui -config file.json` reads the settings. `configuration.nix`
generates this file into the Nix store from the `deskuiConfig` attrset. It
covers the Home Assistant URL, the button and sensor entities, the weather
entity, the notify event, the calendar feeds and the night mode hours.

The Nix store is world-readable, so secrets are never put in it: the config
only references them as `$VARS`, and their values come from files on the
device that are not in this repo:

| File on the Pi | Contents |
|---|---|
| `/var/lib/deskui/secrets.env` (mode 600) | `DESKUI_HA_TOKEN=...` (HA profile → Security → Long-lived access tokens) <br> `DESKUI_ICAL_PERSONAL=https://calendar.google.com/calendar/ical/.../basic.ics` |
| `/var/lib/secrets/wireless.env` (root:wpa_supplicant, 640) | `psk_nojcrib=...` |

A missing deskui secret only disables that source; the display still
starts. Restart with `systemctl restart deskui` after editing.

### Notifications from Home Assistant

Fire the configured event (default `deskie_notify`) from an automation or
script to pop up a message on the display:

```yaml
# in an automation's or script's actions
- event: deskie_notify
  event_data:
    title: Washing machine
    message: The laundry is done.
```

### Night mode

The panel's backlight is wired always-on (there is no `/sys/class/backlight`
device), so night mode can't dim it: it shows a black screen with a dim
clock instead. It turns on

- during the `night.start`-`night.end` hours (may wrap past midnight),
- while `night.away_entity` says nobody is home (state `0`, `not_home`,
  `away` or `off`; e.g. `zone.home`, which counts the people in it),
- or by hand with the **Night** button.

A tap wakes the screen. Manual night mode then stays off; automatic night
mode comes back after `wake_seconds` without a touch.

## Development

```sh
nix develop                      # go, gopls, delve, golangci-lint, build-for-pi
cd app
go test ./...                    # unit tests (widgets, sources, config, rendering)
go run . -backend kitty          # run the UI in the kitty terminal; q quits
DESKUI_SNAPSHOT_DIR=/tmp go test -run Snapshot .   # render screens to PNG
```

In the kitty backend, log output on stderr garbles the picture; redirect it
(`2>deskui.log`).

When Go dependencies change, update `vendorHash` in `flake.nix`: set it to
an empty or fake hash, build, and copy the hash Nix reports.

## Deploying

Build on a machine that can build aarch64 (natively, or x86 with
`boot.binfmt.emulatedSystems = [ "aarch64-linux" ];`) and switch the Pi
over SSH:

```sh
nixos-rebuild switch --flake .#deskie --target-host root@<pi-address>
```

A first install is an SD card image: `nix build .#sdImage`. After flashing
it, create the secret files above; wifi needs `wireless.env`, ethernet works
without it.

## Touch calibration

The touch controller's axes don't match the screen: they are swapped and
mirrored, and the edges sit under the bezel. Raw readings are mapped with an
affine transform passed as `-touch-cal` (set in `configuration.nix`). To
recalibrate, e.g. after changing the panel rotation in `display.nix`:

```sh
systemctl stop deskui
deskui -backend fb -calibrate      # tap the five crosshairs
systemctl start deskui
```

Then put the printed `-touch-cal=...` into `deskuiTouch` in
`configuration.nix` and redeploy.

## Credits

Uses the [Fira Code](https://github.com/tonsky/FiraCode) font (SIL Open Font
License), embedded from `app/fonts/` (static Regular and Bold TTFs).
