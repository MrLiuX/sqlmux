# e2e/release.sh — helpers of f11.1 / f11.3 (M11). Source after lib.sh; expects $D (the script's temp dir).
# A clean clone of this tree's HEAD, tagged v9.9.9, is what GoReleaser builds (the real path of a tag: .Tag, the changelog
# since the last tag) without publishing — nothing leaves the machine, no token is needed. GORELEASER names the binary.
GR=${GORELEASER:-goreleaser}
command -v "$GR" >/dev/null || { echo "e2e: no goreleaser on PATH (set GORELEASER)" >&2; exit 1; }
TAG=v9.9.9
# m99 stands for a milestone's tag (F11.1: no release): it sits on the tested commit and an empty docs: commit comes after, so the changelog only
# lists what it should if GoReleaser skips m* tags and goes back to the last v*.
rel_clone() { SRC=$D/src; git clone -q "$E2E_ROOT" "$SRC" && git -C "$SRC" tag m99 && git -C "$SRC" -c user.name=e2e -c user.email=e2e@example.com commit -q --allow-empty -m "docs: after m99" && git -C "$SRC" tag "$TAG"; }
rel_run()   { (cd "$SRC" && "$GR" release --clean --skip=publish >"$D/gr.log" 2>&1) || { tail -20 "$D/gr.log"; false; }; }
# the binary GoReleaser built for this machine
rel_native() { local os cpu; os=$(uname -s | tr A-Z a-z); cpu=$(uname -m | sed 's/x86_64/amd64/'); echo "$SRC"/dist/sqlmux_${os}_${cpu}_*/sqlmux; }
