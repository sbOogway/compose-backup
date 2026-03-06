PROJECT=test && \
mkdir -p /tmp/${PROJECT}_restore && \
tar xzf ${PROJECT}_backup.tar.gz -C /tmp/${PROJECT}_restore && \
docker load -i /tmp/${PROJECT}_restore/images.tar && \
for f in /tmp/${PROJECT}_restore/*.tar.gz; do
  name=$(basename $f .tar.gz)
  [ "$name" = "images" ] && continue
  docker run --rm \
    -v ${name}:/data \
    -v /tmp/${PROJECT}_restore:/backup \
    alpine tar xzf /backup/${name}.tar.gz -C / --strip-components=1
done && \
COMPOSE_FILE=$(cat /tmp/${PROJECT}_restore/compose.yaml | grep -m1 'name:' | awk '{print $2}') && \
mkdir -p ~/restored_${PROJECT} && \
cp /tmp/${PROJECT}_restore/compose.yaml ~/restored_${PROJECT}/compose.yaml && \
cd ~/restored_${PROJECT} && \
docker compose up -d && \
rm -rf /tmp/${PROJECT}_restore && \
echo "Done: stack restored"