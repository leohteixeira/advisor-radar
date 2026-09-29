#!/bin/sh
# Regenerate every proto/*/v1/*.proto into gen/ with the pinned toolchain.
#
# The output matches the checked-in gen/ layout: gen/<svc>/v1/<svc>.pb.go and
# gen/<svc>/v1/<svc>_grpc.pb.go, with "source: proto/<svc>/v1/<svc>.proto" in
# the generated headers. Rerunning it leaves gen/ byte-identical.
set -eu

want_protoc='libprotoc 29.3'
want_go='protoc-gen-go v1.36.5'
want_grpc='protoc-gen-go-grpc 1.5.1'
module='github.com/leohteixeira/advisor-radar'

# The plugins live in the Go install dir (GOBIN, else the first GOPATH entry
# plus /bin, which is ~/go/bin in the Dev Container and not on PATH there).
command -v go >/dev/null 2>&1 || { printf 'gen-proto: go not found on PATH\n' >&2; exit 1; }
gobin="$(go env GOBIN)"
if [ -z "${gobin}" ]; then
	gopath="$(go env GOPATH)"
	gobin="${gopath%%:*}/bin"
fi
PATH="${gobin}:${PATH}"
export PATH

root="$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)"
cd "${root}"

fail() {
	printf 'gen-proto: %s\n' "$1" >&2
	exit 1
}

check() {
	tool="$1"
	want="$2"
	command -v "${tool}" >/dev/null 2>&1 || fail "${tool} not found on PATH; install ${want}"
	got="$("${tool}" --version 2>&1)" || fail "${tool} --version failed"
	[ "${got}" = "${want}" ] || fail "${tool} --version is '${got}', want '${want}'"
}

check protoc "${want_protoc}"
check protoc-gen-go "${want_go}"
check protoc-gen-go-grpc "${want_grpc}"

# Sorted glob expansion keeps the invocation deterministic.
set -- proto/*/v1/*.proto
[ -e "$1" ] || fail "no proto files under proto/*/v1"

protoc -I . \
	--go_out=. --go_opt="module=${module}" \
	--go-grpc_out=. --go-grpc_opt="module=${module}" \
	"$@"

printf 'gen-proto: generated %d file(s) into gen/\n' "$#"
