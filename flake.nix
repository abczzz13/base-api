{
  description = "Nix development environment for base-api";

  nixConfig = {
    bash-prompt-prefix = "";
    bash-prompt-suffix = "";
  };

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { nixpkgs, ... }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];

      forAllSystems =
        f:
        nixpkgs.lib.genAttrs systems (
          system:
          f {
            pkgs = import nixpkgs { inherit system; };
          }
        );
    in
    {
      devShells = forAllSystems (
        { pkgs }:
        {
          default = pkgs.mkShell {
            packages = with pkgs; [
              actionlint
              curl
              gh
              gitleaks
              gotools
              go_1_26
              gofumpt
              golangci-lint
              goose
              govulncheck
              jq
              just
              ogen
              python3
              shellcheck
              sqlc
              yamlfmt
            ];
          };
        }
      );

      formatter = forAllSystems ({ pkgs }: pkgs.nixfmt);
    };
}
