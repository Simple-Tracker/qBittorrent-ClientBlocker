#!/bin/sh

if [ -f "./config.json" ]; then
    echo "config.json exist"
else
    echo "config.json not exist, generate config from env"

    # Convert $blockList to json array
    tmpBlockList="[]"
    tmpIPBlockList="[]"
    if [ -z "$blockList" ]; then
        tmpBlockList=$(echo $blockList | jq '.')
    fi
    if [ -z "$ipBlockList" ]; then
        tmpIPBlockList=$(echo $ipBlockList | jq '.')
    else

    envKVPair=$(jq -n 'env|to_entries[]')

    # Keep username and password string
    # Keep blockList json array
    # Convert "true" to true, "false" to false, digital string to number
    configKVPair=$(echo $envKVPair | jq --argjson tmpBlockList "$tmpBlockList" --argjson tmpIPBlockList "$tmpIPBlockList" '{
        (.key): (
            if (.key|ascii_downcase) == "qbusername" or (.key|ascii_downcase) == "qbpassword" then .value
            elif (.key|ascii_downcase) == "blocklist" then $tmpBlockList
            elif (.key|ascii_downcase) == "ipblocklist" then $tmpIPBlockList
            else .value|(
                if . == "true" then true
                elif . == "false" then false
                else (tonumber? // .)
                end)
            end
        )
    }')

    (echo $configKVPair | jq -s add) > config.json
fi

exec ./qBittorrent-ClientBlocker
