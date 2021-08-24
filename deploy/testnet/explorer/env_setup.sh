#!/bin/bash

echo "----- Setup environment variables -----"
ls -la

# shellcheck disable=SC2039
if [[ ! $GEV_RPCPORT ]]; then
  GEV_RPCPORT="22001"
fi

sed -i -e \
  's/ENV_GEV_RPCPORT/"'$GEV_RPCPORT'"/g' \
  ./app/app.js

sed -i -e \
  's/ENV_GEV_HOSTNAME/"'$GEV_HOSTNAME'"/g' \
  ./app/app.js

npm start