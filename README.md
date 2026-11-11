# display-wake

A launchd agent for waking a sleeping Mac display when a monitor reconnects, so a monitor that keeps reconnecting can rest.

## Get started

Needs macOS and Go 1.26 or later.

```sh
go install github.com/seriouslysean/display-wake/cmd/display-wake@latest
display-wake install      # writes ~/Library/LaunchAgents/display-wake.plist and loads it
```

What it did goes to `~/Library/Logs/display-wake.log`. To remove it:

```sh
display-wake uninstall
```

Working on it: `scripts/check` runs everything CI runs.

## Docs

- [How it works](docs/how-it-works.md): the reconnect loop, what display-wake does about it, and its limits.

## License

MIT, in [LICENSE](LICENSE).
