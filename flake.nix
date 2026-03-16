{
  description = "Horcrux – split secrets with erasure coding and distribute shards across backends";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};

        version =
          if self ? shortRev then "0.1.0+${self.shortRev}"
          else "0.1.0-dirty";
      in
      {
        packages = {
          default = pkgs.buildGoModule {
            pname = "hrcx";
            inherit version;
            src = ./.;
            vendorHash = "sha256-rd/PINLQxkeDC1ZP+itZtpUqtGkx3YDoF03mE6Ppxys=";

            postInstall = ''
              mv $out/bin/horcrux $out/bin/hrcx
            '';

            ldflags = [
              "-s" "-w"
              "-X github.com/marmos91/horcrux/internal/version.Version=${version}"
            ];

            meta = with pkgs.lib; {
              description = "Split secrets with erasure coding and distribute shards";
              homepage = "https://github.com/marmos91/horcrux";
              license = licenses.mit;
              mainProgram = "hrcx";
            };
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            golangci-lint
            goreleaser
          ];
        };
      });
}
