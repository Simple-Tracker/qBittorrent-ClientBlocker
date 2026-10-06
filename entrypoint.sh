#!/bin/sh

if [ -n "$useENV" ]; then
    echo "Generate config from env."

    # 将 blockList 转为 JSON 数组.
    tmpBlockList='[]'
    tmpIPBlockList='[]'
    if [ -n "$blockList" ]; then
        tmpBlockList=$(echo $blockList | jq '.')
    fi
    if [ -n "$ipBlockList" ]; then
        tmpIPBlockList=$(echo $ipBlockList | jq '.')
    fi

    envKVPair=$(jq -n 'env|to_entries[]')

    # 保留用户名和密码的字符串类型.
    # 保留 blockList 的 JSON 数组类型.
    # 将 "true" 和 "false" 转为布尔值, 数字字符串转为数值.
    configKVPair=$(echo $envKVPair | jq --argjson tmpBlockList "$tmpBlockList" --argjson tmpIPBlockList "$tmpIPBlockList" '{
        (.key): (
            if (.key|ascii_downcase) == "clientusername" or (.key|ascii_downcase) == "clientpassword" then .value
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

    (echo $configKVPair | jq -s add) > config_additional.json
fi

commandArgStr=''
if [ -n "$configPath" ]; then
    commandArgStr="-c $configPath"
fi
if [ -n "$additionalConfigPath" ]; then
    commandArgStr="$commandArgStr -ca $additionalConfigPath"
fi
exec ./qBittorrent-ClientBlocker $commandArgStr
