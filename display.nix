# Waveshare 4inch RPi LCD (A) on a Pi 3, via the nixos-hardware profile.
#
# Verified on hardware: Waveshare's own `waveshare35a` overlay (fbtft driver)
# lights the display and touch on the downstream Raspberry Pi kernel. The stock
# `piscreen,drm` overlay bound the DRM driver but never showed anything.
#
# The overlay is a vendor binary that is NOT in Raspberry Pi's firmware, so it is
# vendored in this repo (overlays/waveshare35a.dtbo, copied from a working card).
{ lib, ... }:
{
  hardware.raspberry-pi.firmware = {
    enable = true;      # on `switch`, only acts if /boot/firmware is mounted
    uboot.enable = true;
  };

  # Keep the kernel console (fbcon) off the panel. Without HDMI the panel is the
  # only framebuffer, so fbcon takes it and draws boot messages, the tty1 login
  # prompt and a cursor over deskui. fbcon=map:N picks which fb the consoles go
  # to; fb9 never exists here, so fbcon binds to no framebuffer at all, whatever
  # number the panel gets. Console access stays on SSH and the serial port
  # (console=ttyS0 from the SD image profile). Side effect: no text console on
  # HDMI either; drop this line to get it back for debugging.
  boot.kernelParams = [ "fbcon=map:9" ];

  # The pinned Raspberry Pi kernel doesn't build ZFS; the base image profile would try.
  boot.supportedFilesystems.zfs = lib.mkForce false;

  hardware.raspberry-pi.configtxt.settings.all = {
    dtparam = [ "spi=on" ];
    dtoverlay = [
      # rotate: 0/90/180/270. speed: SPI clock in Hz (vendor default is 16 MHz).
      # fps: fbtft flush rate. If you see speckles or shifted rows, lower speed.
      "waveshare35a:rotate=90,speed=32000000,fps=50"
    ];
  };

  # The firmware module replaces config.txt and the overlays/ directory on every
  # `switch` (when the partition is mounted) and deletes overlay files it doesn't
  # know about, so a hand-copied waveshare35a.dtbo would be removed.
  # Guess: it copies overlays from pkgs.raspberrypifw. If so, adding the file there
  # makes it persist. Check `ls /boot/firmware/overlays | grep waveshare` after a switch.
  nixpkgs.overlays = [
    (final: prev: {
      raspberrypifw = prev.raspberrypifw.overrideAttrs (old: {
        postFixup = (old.postFixup or "") + ''
          cp ${./overlays/waveshare35a.dtbo} $out/share/raspberrypi/boot/overlays/waveshare35a.dtbo
        '';
      });
    })
  ];
}
