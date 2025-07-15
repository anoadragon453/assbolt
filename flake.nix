{
  description = "A simple, local passbolt UI";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    systems.url = "github:nix-systems/default";
  };

  outputs = { self, nixpkgs, systems }:
    let
      forEachSystem = nixpkgs.lib.genAttrs (import systems);
    in
    {
      devShells = forEachSystem (system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        {
          default = pkgs.mkShell {
            buildInputs = with pkgs; [
	      go
	      mesa
              pkg-config # lets CGO find libs

              # OpenGL
              libGL.dev
	      mesa
	      mesa-gl-headers
	      glfw
             
              # X11 stack – needed if you compile with default GLFW tags
              xorg.libX11.dev
              xorg.libXi.dev
              xorg.libXcursor.dev
              xorg.libXrandr.dev
              xorg.libXinerama.dev
              xorg.libXext.dev
              xorg.libXrender.dev
              xorg.libXfixes.dev     # cursor hiding, etc.
	      xorg.libXxf86vm.dev
             
              # Wayland support (compile with `-tags wayland`)
              wayland
              wayland-protocols
              libxkbcommon
            ];

	    CGO_ENABLED = "1";

	    shellHook = ''
	      # Load environment variables from `.env`
	      eval "$(grep -v '^#' .env | sed -E 's/^/export /')"
	    '';
          };

        }
      );
    };
}
