# Logs

App logs are written under a fixed container path and normally bind-mounted to the host.

## Default path

| Location | Path |
|----------|------|
| Container | `/var/lib/sudostream/logs` |
| Sample compose host bind | `./logs` |

## Viewing

```shell
docker compose logs -f app
# and/or
tail -f logs/*.log
```

## Rotation

Logrotate is not shipped inside the image. Rotate on the host.

??? example "sample logrotate config"
    ```text
    /path/to/sudostream/logs/sudostream.log {
        weekly
        rotate 8
        compress
        missingok
        notifempty
        copytruncate
    }
    ```
