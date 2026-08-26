FROM ghcr.io/cnuss/ubuntu:24.04

ARG VERSION
ARG DEBIAN_FRONTEND=noninteractive

# Recommends stay on: the chrome deb pulls fonts and codec libs through them
RUN curl -fsSL https://dl.google.com/linux/linux_signing_key.pub \
    | gpg --dearmor -o /etc/apt/keyrings/google-chrome.gpg \
    && echo "deb [arch=amd64,arm64 signed-by=/etc/apt/keyrings/google-chrome.gpg] https://dl.google.com/linux/chrome/deb/ stable main" \
    > /etc/apt/sources.list.d/google-chrome.list \
    && apt-get update \
    && apt-get install -y google-chrome-stable=${VERSION}-1 \
    && rm -rf /var/lib/apt/lists/* \
    # Suppress the "unsupported command-line flag" banner (--no-sandbox)
    && install -d -m 0755 /etc/opt/chrome/policies/managed \
    && echo '{"CommandLineFlagSecurityWarningsEnabled": false}' \
    > /etc/opt/chrome/policies/managed/no-flag-warnings.json \
    # Hide the xpra html5 client's floating toolbar by default
    && echo 'floating_menu = no' >> /usr/share/xpra/www/default-settings.txt \
    && install -d -m 700 -o user -g user /tmp/xdg-runtime \
    # Draw tabs into the window frame (undecorated) so xpra adds no title bar
    && install -d -o user -g user /home/user/.config /home/user/.config/google-chrome /home/user/.config/google-chrome/Default \
    && echo '{"browser":{"custom_chrome_frame":true}}' \
    > /home/user/.config/google-chrome/Default/Preferences \
    && chown user:user /home/user/.config/google-chrome/Default/Preferences

COPY --chmod=755 entrypoint.sh /usr/local/bin/entrypoint

ENV XDG_RUNTIME_DIR=/tmp/xdg-runtime

USER user
WORKDIR /workspace
EXPOSE 14500 9222
ENTRYPOINT ["entrypoint"]
