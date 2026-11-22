#!/bin/bash
# DevStack plugin: Glance Go as the image service (devstack@g-go).
# Phases: install (Go toolchain, build), extra (migrate, store, start, Apache
# /image, stop g-api), unstack (stop g-go), clean (remove its private root).
# See docs/glance/devstack-plugin.md.

_XTRACE_GLANCE_GO=$(set +o | grep xtrace)
set -o xtrace

function install_glance_go_toolchain {
    if [[ -x $GLANCE_GO_TOOLCHAIN_DIR/bin/go ]]; then
        return
    fi
    local arch
    case $(uname -m) in
        x86_64) arch=amd64 ;;
        aarch64) arch=arm64 ;;
        *) die $LINENO "Glance Go: unsupported architecture $(uname -m)" ;;
    esac
    local archive
    archive=$(get_extra_file "https://go.dev/dl/go${GLANCE_GO_TOOLCHAIN_VERSION}.linux-${arch}.tar.gz")
    if ! echo "$GLANCE_GO_TOOLCHAIN_SHA256  $archive" | sha256sum --check --status; then
        die $LINENO "Glance Go: Go toolchain checksum mismatch; set GLANCE_GO_TOOLCHAIN_SHA256"
    fi
    local tmp
    tmp=$(mktemp -d)
    tar -C "$tmp" -xzf "$archive"
    sudo rm -rf "$GLANCE_GO_TOOLCHAIN_DIR"
    sudo mv "$tmp/go" "$GLANCE_GO_TOOLCHAIN_DIR"
    rmdir "$tmp"
    sudo chown -R "$STACK_USER" "$GLANCE_GO_TOOLCHAIN_DIR"
}

# install_glance_go: a static binary from this checkout, no system packages.
function install_glance_go {
    install_glance_go_toolchain
    (cd "$GLANCE_GO_DIR" && CGO_ENABLED=0 GOTOOLCHAIN=local \
        "$GLANCE_GO_TOOLCHAIN_DIR/bin/go" build -trimpath -buildvcs=false -o "$GLANCE_GO_BIN" ./cmd/glance)
}

# glance_go_env: the unit's environment. Database, catalog, store directory,
# Keystone and notifications come from the original's glance-api.conf.
function glance_go_env {
    echo "GLANCE_GO_CONFIG_FILE=$GLANCE_API_CONF GLANCE_GO_LISTEN_ADDR=$GLANCE_GO_LISTEN" \
        "GLANCE_GO_BINARY_ENABLED=true GLANCE_GO_STORE_ROOT=$GLANCE_GO_STORE_ROOT $GLANCE_GO_EXTRA_ENV"
}

# configure_glance_go: its bookkeeping database next to glance (never the
# original's tables) and the private store root, bound to that database.
function configure_glance_go {
    local env
    env=$(glance_go_env)
    install -d -m 700 "$GLANCE_GO_STORE_ROOT"
    env $env "$GLANCE_GO_BIN" migrate
    local scope
    scope=$(env $env "$GLANCE_GO_BIN" store-scope)
    if [[ -z "$(ls -A "$GLANCE_GO_STORE_ROOT")" ]]; then
        env $env GLANCE_GO_STORE_SCOPE=$scope "$GLANCE_GO_BIN" store-init
    fi
    GLANCE_GO_SCOPE=$scope
}

function start_glance_go {
    run_process g-go "$GLANCE_GO_BIN serve" "" "" "$(glance_go_env) GLANCE_GO_STORE_SCOPE=$GLANCE_GO_SCOPE"
    if ! wait_for_service $SERVICE_TIMEOUT "http://$GLANCE_GO_LISTEN/v2/images"; then
        die $LINENO "g-go did not start"
    fi
    # /image now reaches Glance Go; the Keystone endpoint stays the same.
    write_local_proxy_http_config glance-api "http://$GLANCE_GO_LISTEN" "/image"
    stop_process g-api
    $SYSTEMCTL disable devstack@g-api.service
    if ! wait_for_service $SERVICE_TIMEOUT "$GLANCE_URL"; then
        die $LINENO "/image did not answer through Glance Go"
    fi
}

if is_service_enabled g-go; then
    if [[ "$1" == "stack" && "$2" == "install" ]]; then
        if ! is_service_enabled g-api; then
            die $LINENO "g-go needs g-api enabled: DevStack configures and migrates Glance through it"
        fi
        echo_summary "Installing Glance Go"
        install_glance_go
    elif [[ "$1" == "stack" && "$2" == "extra" ]]; then
        echo_summary "Starting Glance Go in place of g-api"
        configure_glance_go
        start_glance_go
    fi
    if [[ "$1" == "unstack" ]]; then
        stop_process g-go
    fi
    if [[ "$1" == "clean" ]]; then
        sudo rm -rf "$GLANCE_GO_STORE_ROOT"
    fi
fi

$_XTRACE_GLANCE_GO
