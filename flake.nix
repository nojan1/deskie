{
  description = "Desk display: Raspberry Pi 3 + Waveshare 4inch RPi LCD (A), NixOS + Go";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    nixos-hardware = {
      url = "github:NixOS/nixos-hardware/master";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, nixos-hardware }:
    let
      forAll = nixpkgs.lib.genAttrs [ "x86_64-linux" "aarch64-linux" ];
    in
    {
      # The Go UI, available to the system config as pkgs.deskui
      overlays.default = final: prev: {
        deskui = final.buildGoModule {
          pname = "deskui";
          version = "0.1.0";
          src = ./app;
          vendorHash = "sha256-t/mPauAt/jmiK3thoSclM+5GNEVvQcQ/JWfWIWL40Rg=";
        };
      };

      nixosConfigurations.deskie = nixpkgs.lib.nixosSystem {
        system = "aarch64-linux";
        modules = [
          "${nixpkgs}/nixos/modules/installer/sd-card/sd-image-aarch64.nix"
          nixos-hardware.nixosModules.raspberry-pi-3
          { nixpkgs.overlays = [ self.overlays.default ]; }
          ./configuration.nix
          ./display.nix
        ];
      };

      devShells = forAll (system:
        let pkgs = nixpkgs.legacyPackages.${system}; in {
          default = let
            linuxBuild = pkgs.writeShellScriptBin "build-for-pi" ''
              GOOS=linux GOARCH=arm64 go build -o deskui .
            '';
          in pkgs.mkShell {
            packages = with pkgs; [ linuxBuild go gopls gotools delve golangci-lint ];
          };
        });

      packages = forAll (system: {
        deskui = (import nixpkgs {
          inherit system;
          overlays = [ self.overlays.default ];
        }).deskui;
        # nix build .#sdImage   (on x86 you need boot.binfmt.emulatedSystems = [ "aarch64-linux" ])
        sdImage = self.nixosConfigurations.deskie.config.system.build.sdImage;
      });
    };
}
