{ pkgs, ... }:
{
  networking.hostName = "deskie";
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

  environment.systemPackages = with pkgs; [ btop evtest libinput usbutils ];

  # Service user: needs /dev/fbN (video) and /dev/input/event* (input)
  users.users.deskui = {
    isSystemUser = true;
    group = "deskui";
    extraGroups = [ "video" "input" ];
  };
  users.groups.deskui = { };

  systemd.services.deskui = {
    wantedBy = [ "multi-user.target" ];
    after = [ "systemd-modules-load.service" ];
    serviceConfig = {
      ExecStart = "${pkgs.deskui}/bin/deskui";
      User = "deskui";
      Restart = "on-failure";
      RestartSec = 2;
    };
  };
}
