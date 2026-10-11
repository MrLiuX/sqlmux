# e2e/release.sh — helpers of f11.1 / f11.3 (M11). Source after lib.sh; expects $D (the script's temp dir).
# A clean clone of this tree's HEAD, tagged v9.9.9, is what GoReleaser builds (the real path of a tag: .Tag, the changelog
# since the last tag) without publishing — nothing leaves the machine, no token is needed. GORELEASER names the binary.
GR=${GORELEASER:-goreleaser}
command -v "$GR" >/dev/null || { echo "e2e: no goreleaser on PATH (set GORELEASER)" >&2; exit 1; }
TAG=v9.9.9
rel_clone() { SRC=$D/src; git clone -q "$E2E_ROOT" "$SRC" && git -C "$SRC" tag "$TAG"; }
rel_run()   { (cd "$SRC" && "$GR" release --clean --skip=publish >"$D/gr.log" 2>&1) || { tail -20 "$D/gr.log"; false; }; }
# the binary GoReleaser built for this machine
rel_native() { local os cpu; os=$(uname -s | tr A-Z a-z); cpu=$(uname -m | sed 's/x86_64/amd64/'); echo "$SRC"/dist/sqlmux_${os}_${cpu}_*/sqlmux; }
