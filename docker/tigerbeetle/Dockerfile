# The upstream image (alpine + tini + the tigerbeetle binary) has no notion of
# "format if needed, then start" — it just execs whatever command you pass it.
# We wrap it with a small entrypoint so `docker compose up` can create the data
# file on first run instead of requiring a separate manual `format` step.
FROM ghcr.io/tigerbeetle/tigerbeetle:0.17.9

COPY entrypoint.sh /entrypoint.sh

ENTRYPOINT ["tini", "--", "/entrypoint.sh"]
