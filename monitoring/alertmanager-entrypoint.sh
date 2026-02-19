#!/bin/sh
set -e

# Substitute env vars in alertmanager config (envsubst not available in busybox)
sed -e "s|PLACEHOLDER_TELEGRAM_BOT_TOKEN|${TELEGRAM_BOT_TOKEN}|g" \
    -e "s|PLACEHOLDER_TELEGRAM_CHAT_ID|${TELEGRAM_CHAT_ID}|g" \
    /etc/alertmanager/alertmanager.template.yml > /etc/alertmanager/alertmanager.yml

exec /bin/alertmanager "$@"
