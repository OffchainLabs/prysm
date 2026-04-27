#!/bin/bash

# Note: The STABLE_ prefix will force a relink when the value changes when using rules_go x_defs.

echo STABLE_GIT_COMMIT "$(git rev-parse HEAD)"
echo DATE "$(date -u +"%Y-%m-%d %H:%M:%S+00:00")"
echo DATE_UNIX "$(date -u +%s)"
echo DOCKER_TAG "$(git rev-parse --abbrev-ref HEAD)-$(git rev-parse --short=6 HEAD)"
echo STABLE_GIT_TAG "$(git describe --tags --abbrev=0)"
