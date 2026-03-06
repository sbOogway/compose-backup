#!/bin/bash

set -e

COMPOSE_PATH=""
BACKUP_NAME=""
MODE=""

usage() {
    cat << EOF
Usage: $(basename "$0") [backup|restore] -p <compose-path> [-n <backup-name>]

Options:
    backup              Create a backup
    restore             Restore from a backup
    -p, --path          Path to docker compose directory (required for backup)
    -n, --name          Backup file name (default: compose-backup-YYYYMMDD-HHMMSS.tar.gz)
    -h, --help          Show this help

Examples:
    $(basename "$0") backup -p /opt/nextcloud
    $(basename "$0") backup -p /opt/nextcloud -n my-backup
    $(basename "$0") restore -n my-backup.tar.gz
EOF
    exit 1
}

parse_args() {
    if [[ $# -lt 2 ]]; then
        usage
    fi

    MODE="$1"
    shift

    while [[ $# -gt 0 ]]; do
        case "$1" in
            -p|--path)
                COMPOSE_PATH="$2"
                shift 2
                ;;
            -n|--name)
                BACKUP_NAME="$2"
                shift 2
                ;;
            -h|--help)
                usage
                ;;
            *)
                echo "Unknown option: $1"
                usage
                ;;
        esac
    done
}

validate_backup_mode() {
    if [[ -z "$COMPOSE_PATH" ]]; then
        echo "Error: Compose path is required for backup"
        usage
    fi

    if [[ ! -d "$COMPOSE_PATH" ]]; then
        echo "Error: Directory does not exist: $COMPOSE_PATH"
        exit 1
    fi

    if [[ ! -f "$COMPOSE_PATH/docker-compose.yml" ]] && [[ ! -f "$COMPOSE_PATH/docker-compose.yaml" ]]; then
        echo "Error: No docker-compose.yml or docker-compose.yaml found in $COMPOSE_PATH"
        exit 1
    fi
}

validate_restore_mode() {
    if [[ -z "$BACKUP_NAME" ]]; then
        echo "Error: Backup name is required for restore"
        usage
    fi

    if [[ ! -f "$BACKUP_NAME" ]]; then
        echo "Error: Backup file not found: $BACKUP_NAME"
        exit 1
    fi
}

backup() {
    local compose_dir="$COMPOSE_PATH"
    local backup_file="${BACKUP_NAME:-compose-backup-$(date +%Y%m%d-%H%M%S).tar.gz}"
    local temp_dir

    temp_dir=$(mktemp -d)
    trap "rm -rf $temp_dir" EXIT

    echo "Backing up docker compose from: $compose_dir"

    cd "$compose_dir"

    local compose_file=""
    if [[ -f "docker-compose.yml" ]]; then
        compose_file="docker-compose.yml"
    elif [[ -f "docker-compose.yaml" ]]; then
        compose_file="docker-compose.yaml"
    fi

    echo "Extracting images..."
    docker compose -f "$compose_file" config --images > "$temp_dir/images.txt"

    local images=()
    while IFS= read -r image; do
        [[ -n "$image" ]] && images+=("$image")
    done < "$temp_dir/images.txt"

    if [[ ${#images[@]} -gt 0 ]]; then
        echo "Saving ${#images[@]} images..."
        docker save "${images[@]}" -o "$temp_dir/images.tar"
    else
        echo "No images to save"
        touch "$temp_dir/images.tar"
    fi

    echo "Copying compose files..."
    cp "$compose_file" "$temp_dir/docker-compose.yml"
    cp "$compose_file" "$temp_dir/docker-compose.yaml"

    if [[ -f ".env" ]]; then
        cp ".env" "$temp_dir/.env"
    fi

    if [[ -f "docker-compose.override.yml" ]]; then
        cp "docker-compose.override.yml" "$temp_dir/docker-compose.override.yml"
    fi
    if [[ -f "docker-compose.override.yaml" ]]; then
        cp "docker-compose.override.yaml" "$temp_dir/docker-compose.override.yaml"
    fi

    echo "Creating archive: $backup_file"
    tar -czpvf "$backup_file" -C "$temp_dir" .

    echo "Backup complete: $backup_file"
}

restore() {
    local backup_file="$BACKUP_NAME"
    local temp_dir

    temp_dir=$(mktemp -d)
    trap "rm -rf $temp_dir" EXIT

    echo "Restoring from: $backup_file"

    tar -xvpzf "$backup_file" -C "$temp_dir"

    local compose_dir
    compose_dir=$(pwd)

    if [[ -f "$temp_dir/docker-compose.yml" ]]; then
        cp "$temp_dir/docker-compose.yml" "$compose_dir/"
    fi

    if [[ -f "$temp_dir/.env" ]]; then
        cp "$temp_dir/.env" "$compose_dir/"
    fi

    if [[ -f "$temp_dir/docker-compose.override.yml" ]]; then
        cp "$temp_dir/docker-compose.override.yml" "$compose_dir/"
    fi

    echo "Loading docker images..."
    if [[ -s "$temp_dir/images.tar" ]]; then
        docker load -i "$temp_dir/images.tar"
    else
        echo "No images to load"
    fi

    echo "Restore complete. Compose files are in: $compose_dir"
    echo "To start services, run: cd $compose_dir && docker compose up -d"
}

main() {
    parse_args "$@"

    case "$MODE" in
        backup)
            validate_backup_mode
            backup
            ;;
        restore)
            validate_restore_mode
            restore
            ;;
        *)
            echo "Error: Invalid mode. Use 'backup' or 'restore'"
            usage
            ;;
    esac
}

main "$@"
