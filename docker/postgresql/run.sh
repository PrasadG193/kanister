#!/bin/bash
# Forked from https://github.com/bitnami/bitnami-docker-postgresql/blob/master/11/debian-9/rootfs/run.sh 
# to modify runtime args

set -o errexit
set -o nounset
set -o pipefail
# set -o xtrace # Uncomment this line for debugging purpose
# shellcheck disable=SC1091

# Load libraries
. /libpostgresql.sh
. /libos.sh

# Load PostgreSQL environment variables
eval "$(postgresql_env)"
readonly flags=("-D" "$POSTGRESQL_DATA_DIR" "--config-file=$POSTGRESQL_CONF_FILE" "--external_pid_file=$POSTGRESQL_PID_FILE" "--hba_file=$POSTGRESQL_PGHBA_FILE" "-c" "archive_command=envdir \"${PGDATA}/env\" wal-e wal-push %p" "-c" "archive_mode=true" "-c" "archive_timeout=60" "-c" "wal_level=archive")
readonly cmd=$(command -v postgres)

info "** Starting PostgreSQL **"
if am_i_root; then
    exec gosu "$POSTGRESQL_DAEMON_USER" "${cmd}" "${flags[@]}"
else
    exec "${cmd}" "${flags[@]}"
fi