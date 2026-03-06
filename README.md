# compose-backup

Backup & restore Docker Compose stacks.

## Status

**Work in progress** — This is an experiment to add a native backup/restore feature to Docker Compose.

## Motivation

Docker lacks a built-in backup system for compose stacks. This tool aims to fill that gap by providing:

```bash
docker compose backup <stack-name>
docker compose restore <stack-name> <backup.tar.gz>
```

## Proof of Concept

`docker-compose-backup` is a shell script that handles **bind volumes only**. It serves as the starting point for the Go implementation.

### How it works

1. **Backup**: Stops containers → saves images → creates tar.gz → restarts containers
2. **Restore**: Extracts tar.gz → loads images → starts containers

### Usage

```bash
./docker-compose-backup backup myapp /opt/myapp
./docker-compose-backup restore myapp /opt/myapp
```

## Go Implementation
### WARNING THIS IS AI TRASH 

The `main.go` file contains a Go implementation that:
- Backs up compose config
- Backs up Docker images
- Backs up named volumes (via Docker API)
- Backs up bind mounts (from compose config)

This is the basis for the eventual Docker Compose plugin.



### Build

```bash
go build -o docker-compose-backup main.go
mv docker-compose-backup ~/.docker/cli-plugins/docker-compose-backup
chmod +x ~/.docker/cli-plugins/docker-compose-backup
```

### Usage

```bash
docker compose backup my-stack
docker compose restore my-stack backup.tar.gz
```

## Known Issues / Edge Cases

- Multiple compose files not fully supported
- Volume backup needs improvement
- Bind mount handling needs work

