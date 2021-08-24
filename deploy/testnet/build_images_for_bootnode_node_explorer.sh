#!/bin/bash

version=
until [[ $version ]]; do read -rp "- Tag Version/Branch Name you want to build: " version; done
env=
until [[ $env ]]; do read -rp "- Environment of Image: " env; done
buildExplorer=
until [[ $buildExplorer ]]; do read -rp "- Do you want to build Explorer Image? (y/n) " buildExplorer; done

# Replace / with -
newVersion=${version//\//-}

BASEDIR=$(dirname "$0")
BUILDER_REPOSITORY="kybernetwork/evrynet-builder"
BUILDER_TAG_ENV="$BUILDER_REPOSITORY:$newVersion-$env"

BOOTNODE_REPOSITORY="kybernetwork/evrynet-bootnode"
BOOTNODE_TAG_ENV="$BOOTNODE_REPOSITORY:$newVersion-$env"

NODE_REPOSITORY="kybernetwork/evrynet-node"
NODE_TAG_ENV="$NODE_REPOSITORY:$newVersion-$env"

EXPLORER_REPOSITORY="kybernetwork/evrynet-explorer"
EXPLORER_TAG_ENV="$EXPLORER_REPOSITORY:$newVersion-$env"

git fetch --all --tags --prune
rm -rf "$BASEDIR"/builder/project
if [ "$version" == "develop" ];
then
  echo "--- Cloning evrynet-node from develop branch ..."
  git clone --single-branch -b develop --single-branch git@github.com:Evrynetlabs/evrynet-node.git "$BASEDIR"/builder/project
else
  echo "--- Cloning evrynet-node from tag $version ..."
  git clone -b "$version" git@github.com:Evrynetlabs/evrynet-node.git "$BASEDIR"/builder/project
fi


echo "--- Building builder container for version $version"
yes | sudo docker build -f ./deploy/testnet/builder/Dockerfile -t "$BUILDER_TAG_ENV" "$BASEDIR"/builder/

echo "--- Creating temporary docker container from $BUILDER_TAG_ENV to get bin files"
TempBuilderContainer=$(sudo docker create "$BUILDER_TAG_ENV")

echo "--- Copy gev, bootnode from $BUILDER_TAG_ENV to ./nodes/bin/, ./bootnode/bin/"
rm -rf "$BASEDIR"/nodes/bin/gev
rm -rf "$BASEDIR"/bootnode/bin/bootnode
mkdir -p "$BASEDIR"/nodes/bin/ "$BASEDIR"/bootnode/bin/
sudo docker cp "$TempBuilderContainer":/evrynet/gev "$BASEDIR"/nodes/bin/
sudo docker cp "$TempBuilderContainer":/evrynet/bootnode "$BASEDIR"/bootnode/bin/
sudo docker rm -v "$TempBuilderContainer"

echo "--- Building bootnode image $BOOTNODE_TAG_ENV "
yes | sudo docker build -f "$BASEDIR"/bootnode/Dockerfile -t "$BOOTNODE_TAG_ENV" "$BASEDIR"/bootnode/

echo "--- Building node image $NODE_TAG_ENV "
yes | sudo docker build -f "$BASEDIR"/nodes/Dockerfile -t "$NODE_TAG_ENV" "$BASEDIR"/nodes/

rm -rf "$BASEDIR"/builder/project


if [[ "$buildExplorer" == "y" ]]; then
  rm -rf "$BASEDIR"/explorer/web
  echo "--- Cloning explorer from develop branch ..."
  git clone git@github.com:Evrynetlabs/explorer.git "$BASEDIR"/explorer/web

  sudo docker rmi -f $EXPLORER_TAG_ENV
  sudo docker rm -f gev-explorer

  echo "--- Building explorer image $EXPLORER_TAG_ENV "
  yes | sudo docker build -f "$BASEDIR"/explorer/Dockerfile -t "$EXPLORER_TAG_ENV" "$BASEDIR"/explorer

  rm -rf "$BASEDIR"/explorer/web
fi
