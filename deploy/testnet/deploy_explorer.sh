#!/bin/bash

version=
until [[ $version ]]; do read -rp "- Tag Version/Branch Name you want to deploy: " version; done
env=
until [[ $env ]]; do read -rp "- Environment of Image: " env; done
rpcAddr=
until [[ $rpcAddr ]]; do read -rp "- Input RPC Address to connect: " rpcAddr; done
rpcPort=
until [[ $rpcPort ]]; do read -rp "- Input RPC Port to connect: " rpcPort; done

# Replace / with -
newVersion=${version//\//-}

BASEDIR=$(dirname "$0")
EXPLORER_REPOSITORY="kybernetwork/evrynet-explorer"
EXPLORER_TAG_ENV="$EXPLORER_REPOSITORY:$version-$env"

rm -rf "$BASEDIR"/explorer/web
# Already existed image
if [[ "$(sudo docker images -q "$EXPLORER_TAG_ENV" 2>/dev/null)" != "" ]]; then
  echo "=> Image $EXPLORER_TAG_ENV already existed!"
  rebuild=
  until [[ $rebuild ]]; do read -rp "- Do you want to re-build Explorer Image? (y/n) " rebuild; done

  if [[ "$rebuild" == "y" ]]; then
    echo "--- Cloning explorer from master branch ..."
    git clone -b "$version" git@github.com:Evrynetlabs/explorer.git "$BASEDIR"/explorer/web

    echo "--- Removing docker container & image for $EXPLORER_TAG_ENV ..."
    sudo docker rmi -f $EXPLORER_TAG_ENV
    sudo docker rm -f gev-explorer

    echo "--- Building explorer image $EXPLORER_TAG_ENV "
    yes | sudo docker build -f "$BASEDIR"/explorer/Dockerfile -t "$EXPLORER_TAG_ENV" "$BASEDIR"/explorer
  fi
else
  echo "--- Cloning explorer from master branch ..."
  git clone -b "$version" git@github.com:Evrynetlabs/explorer.git "$BASEDIR"/explorer/web

  echo "--- Removing docker container & image for $EXPLORER_TAG_ENV ..."
  sudo docker rmi -f $EXPLORER_TAG_ENV
  sudo docker rm -f gev-explorer

  echo "--- Building explorer image $EXPLORER_TAG_ENV "
  yes | sudo docker build -f "$BASEDIR"/explorer/Dockerfile -t "$EXPLORER_TAG_ENV" "$BASEDIR"/explorer

fi
rm -rf "$BASEDIR"/explorer/web

yes | sudo docker rm -f gev-explorer

echo "--- Starting explorer on image $EXPLORER_TAG_ENV..."
yes | sudo docker run --name gev-explorer -d \
  --publish 8080:8080 \
  -e GEV_RPCPORT=$rpcPort \
  -e GEV_HOSTNAME=$rpcAddr \
  "$EXPLORER_TAG_ENV"