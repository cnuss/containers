#!/bin/sh
# Launches google-chrome inside an xpra session, served to browsers over
# xpra's HTML5 client on TCP 14500, with CDP on 127.0.0.1:9222. Arguments
# pass through to google-chrome.
# NOTE: arguments are re-split by the shell inside --start-child, so arguments
# containing spaces are not supported.
set -eu

exec xpra start :100 \
    --daemon=no \
    --bind-tcp=0.0.0.0:14500 \
    --html=on \
    --resize-display=yes \
    --sharing=yes \
    --exit-with-children=yes \
    --start-child="google-chrome --no-sandbox --no-first-run --disable-gpu --disable-dev-shm-usage --start-maximized --remote-debugging-port=9222 $*"
