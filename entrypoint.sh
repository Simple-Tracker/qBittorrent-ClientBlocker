#!/bin/sh

if [ -f "./config.json" ]; then
    echo "config.json exist"
else
    echo "config.json not exist, generate config from env"

    # convert $blockList to json array
    if [ -z "${blockList}" ]; then
        tempBlockList=$(jq -n --argjson arr "[]" '$arr')
    else
        tempBlockList=$(echo $blockList | jq '.')
    fi

    config=$(jq -n 'env|to_entries[]')

    # keep username and password string
    # keep blockList json array
    # convert "true" to true, "false" to false, digital string to number
    config=$(echo $config | jq --argjson tempBlockList "$tempBlockList" '{
        (.key): (
            if (.key|ascii_downcase) == "qbusername" or (.key|ascii_downcase) == "qbpassword" then .value
            elif (.key|ascii_downcase) == "blocklist" then $tempBlockList
            else .value|(
                if . == "true" then true
                elif . == "false" then false
                else (tonumber? // .)
                end)
            end
        )
    }')

    (echo $config | jq -s add) >config.json
fi

cat config.json

./qBittorrent-ClientBlocker
