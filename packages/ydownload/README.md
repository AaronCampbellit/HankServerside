# YDownload

Installable Hank agent app package `ydownload`.

YDownload runs `yt-dlp` from the home agent, stages completed downloads in a
private temporary directory, and streams the finished files into the configured
Hank file source. The app exposes `/ydownload` and a single `download` command.

The agent host or container must have `yt-dlp` available on `PATH`, or the app
setting `yt_dlp_path` must point to the executable.

Common settings include:

- `source_id`: Hank file source for completed downloads.
- `destination_path`: destination folder inside that source.
- `format`: yt-dlp format selector, default `bv*+ba/b`.
- `output_template`: yt-dlp output template.
- `write_subtitles`, `write_auto_subtitles`, `subtitle_languages`,
  `subtitle_format`: subtitle options.
- `write_thumbnail`, `write_info_json`: sidecar file options.
- `download_playlist`: opt in to playlist downloads.
- `restrict_filenames`, `no_overwrite`: filename and overwrite behavior.
- `rate_limit`, `proxy_url`, `cookies_file_path`: network/auth helpers.
- `timeout_seconds`: maximum runtime for the yt-dlp process.

Build with:

```bash
scripts/package-ydownload-app.sh
```

Import `dist/ydownload.hankapp` from Settings > Apps, then use the installed
app's Configure action. The form is rendered from `app.json`
`config.settings_schema`; do not add a dedicated settings panel for this app.
