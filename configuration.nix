{ pkgs, ... }:
let
  # deskui settings (see app/config.go). This ends up world-readable in the
  # Nix store, so secrets are only referenced as $VARS here; their values go
  # in /var/lib/deskui/secrets.env on the device (mode 600), e.g.
  #   DESKUI_HA_TOKEN=eyJ...            (HA profile > Security > Long-lived tokens)
  #   DESKUI_ICAL_PERSONAL=https://calendar.google.com/calendar/ical/.../basic.ics
  # A missing secret just disables that source.
  deskuiConfig = pkgs.writeText "deskui.json" (builtins.toJSON {
    home_assistant = {
      url = "http://192.168.1.15:8123";
      token = "$DESKUI_HA_TOKEN";
      # Home screen toggles; empty shows local demo toggles.
      buttons = [
        # { label = "Lamp"; entity = "light.desk_lamp"; }
      ];
      # Fire from an automation to pop up a message:
      #   event: deskie_notify, data: { title: "...", message: "..." }
      notify_event = "deskie_notify";
    };
    calendar = {
      feeds = [
        { name = "Personal"; url = "$DESKUI_ICAL_PERSONAL"; }
      ];
      days = 7;
      refresh_minutes = 10;
    };
  });

  # Touch calibration for the ADS7846 with the panel at rotate=90: an affine
  # raw->pixel map (x = a*rx + b*ry + c, y = d*rx + e*ry + f) fitted from five
  # on-screen targets. Max error at the targets: 4 px corners, 8 px centre.
  # To redo it (e.g. after changing the rotation), on the Pi:
  #   systemctl stop deskui; deskui -backend fb -calibrate; systemctl start deskui
  # and paste the printed value here.
  deskuiTouch = "-touch-cal=-0.00035163,-0.129781,513.937,-0.084262,-0.00103614,331.202";
in
{
  networking.hostName = "deskie";
  time.timeZone = "Europe/Stockholm";
  system.stateVersion = "26.04";
  nix.settings.experimental-features = [ "nix-command" "flakes" ];

  hardware.enableRedistributableFirmware = true; # Pi 3 wifi/bt firmware

  # Wifi (fill in, or use ethernet)
  networking.wireless.enable = true;
  networking.wireless.networks."nojcrib".psk = "REMOVED-see-wireless.env";

  services.openssh.enable = true;
  users.users.root.openssh.authorizedKeys.keys = [
    "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII01sWlLj7/5LCdK59vI8w9cgPsI3+IsEWS0doAVV2vX"
  ];

  environment.systemPackages = with pkgs; [ btop evtest libinput usbutils deskui ];

  # Service user: needs /dev/fbN (video) and /dev/input/event* (input)
  users.users.deskui = {
    isSystemUser = true;
    group = "deskui";
    extraGroups = [ "video" "input" ];
  };
  users.groups.deskui = { };

  systemd.services.deskui = {
    wantedBy = [ "multi-user.target" ];
    after = [ "systemd-modules-load.service" "network-online.target" ];
    wants = [ "network-online.target" ];
    serviceConfig = {
      # Explicit fb backend: "auto" falls back to the terminal backend when the
      # panel isn't up yet at boot, which can't work under systemd.
      ExecStart = "${pkgs.deskui}/bin/deskui -backend fb -config ${deskuiConfig} ${deskuiTouch}";
      EnvironmentFile = "-/var/lib/deskui/secrets.env"; # optional, see above
      User = "deskui";
      Restart = "on-failure";
      RestartSec = 2;
    };
  };
}
