# Docker action

The **DOCKER** action runs basic Docker commands from FlowK. Use it to list
images and containers, start or stop containers, fetch logs, run commands inside
a container, and manage volumes or networks.

## Available operations

| Operation | Equivalent command |
| --- | --- |
| `IMAGES_LIST` | `docker images` |
| `IMAGE_PULL` | `docker pull <image>` |
| `IMAGE_REMOVE` | `docker rmi <image>` |
| `IMAGE_PRUNE` | `docker image prune --force` |
| `CONTAINERS_LIST` | `docker ps` |
| `CONTAINERS_LIST_ALL` | `docker ps -a` |
| `CONTAINER_RUN` | `docker run [-i] [-t] [--name <name>] [-e <env>...] [-p <port>...] <image> [command...]` |
| `CONTAINER_START` | `docker start <container>` |
| `CONTAINER_STOP` | `docker stop <container>` |
| `CONTAINER_RESTART` | `docker restart <container>` |
| `CONTAINER_REMOVE` | `docker rm <container>` |
| `CONTAINER_PRUNE` | `docker container prune --force` |
| `CONTAINER_LOGS` | `docker logs <container>` |
| `CONTAINER_EXEC` | `docker exec [-i] [-t] <container> <command...>` |
| `VOLUME_LIST` | `docker volume ls` |
| `VOLUME_CREATE` | `docker volume create <volume>` |
| `VOLUME_INSPECT` | `docker volume inspect <volume>` |
| `VOLUME_REMOVE` | `docker volume rm <volume>` |
| `VOLUME_PRUNE` | `docker volume prune --force` |
| `NETWORK_LIST` | `docker network ls` |
| `NETWORK_CREATE` | `docker network create <network>` |
| `NETWORK_INSPECT` | `docker network inspect <network>` |
| `NETWORK_REMOVE` | `docker network rm <network>` |

## Example

```json
{
  "id": "docker-demo",
  "description": "Basic Docker actions",
  "action": "DOCKER",
  "operation": "CONTAINER_RUN",
  "image": "nginx"
}
```

For interactive mode:

```json
{
  "id": "docker-interactive",
  "description": "Enter Ubuntu",
  "action": "DOCKER",
  "operation": "CONTAINER_RUN",
  "image": "ubuntu",
  "command": ["bash"],
  "interactive": true,
  "tty": true
}
```

With name, env vars, and ports:

```json
{
  "id": "docker-run-flags",
  "description": "Example with name, env, and ports",
  "action": "DOCKER",
  "operation": "CONTAINER_RUN",
  "image": "mysql:8.0",
  "name": "mysql-demo",
  "env": [
    "MYSQL_ROOT_PASSWORD=secret",
    "MYSQL_DATABASE=demo"
  ],
  "ports": ["3307:3306"],
  "command": ["--default-authentication-plugin=mysql_native_password"]
}
```
