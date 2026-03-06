# does not handle overrides or multiple compose config
PROJECT=librechat && \
mkdir -p /tmp/${PROJECT}_backup && \
docker compose -p ${PROJECT} images --format json | python3 -c "import sys,json; [print(i['Repository']+':'+i['Tag']) for i in json.load(sys.stdin)]" | \
  xargs docker save -o /tmp/${PROJECT}_backup/images.tar && \
docker compose -p ${PROJECT} ps -q | head -1 | xargs docker inspect \
  --format '{{index .Config.Labels "com.docker.compose.project.config_files"}}' | \
  xargs -I{} cp {} /tmp/${PROJECT}_backup/compose.yaml && \
docker compose -p ${PROJECT} ps -q | xargs docker inspect \
  --format '{{range .Mounts}}{{if eq .Type "volume"}}{{.Name}} {{end}}{{end}}' | \
  tr ' ' '\n' | sort -u | grep . | \
  xargs -I{} docker run --rm \
    -v {}:/data \
    -v /tmp/${PROJECT}_backup:/backup \
    alpine tar czf /backup/{}.tar.gz /data && \
docker compose -p ${PROJECT} ps -q | xargs docker inspect \
  --format '{{range .Mounts}}{{if eq .Type "bind"}}{{.Source}} {{end}}{{end}}' | \
  tr ' ' '\n' | sort -u | grep . | \
  xargs -I{} docker run --rm \
    -v {}:/data \
    -v /tmp/${PROJECT}_backup:/backup \
    alpine sh -c 'tar czf /backup/$(basename {}).tar.gz /data' && \
tar czf ${PROJECT}_backup.tar.gz -C /tmp/${PROJECT}_backup . && \
rm -rf /tmp/${PROJECT}_backup && \
echo "Done: ${PROJECT}_backup.tar.gz"