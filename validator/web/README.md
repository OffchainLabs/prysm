# Prysm Validator Web UI

This directory contains the files necessary for the Prysm validator web user interface. The UI itself is maintained in a separate repository at [prysmaticlabs/prysm-web-ui](https://github.com/prysmaticlabs/prysm-web-ui).

## Current Status

**Note: The web UI is currently in a frozen state and there is no longer an automated PR workflow for releasing updates.**

This freeze was implemented in [PR #12719](https://github.com/prysmaticlabs/prysm/pull/12719) which removed the build content.

## Updating the Web UI

To update the `site_data.go` file with a new release from the [prysm-web-ui repository](https://github.com/prysmaticlabs/prysm-web-ui/releases), follow these steps:

1. Download and install [go-bindata](https://github.com/kevinburke/go-bindata) (version 4.0.2 confirmed working):
   ```
   go get -u github.com/kevinburke/go-bindata/...
   ```

2. Download the specific release from https://github.com/prysmaticlabs/prysm-web-ui/releases

3. Extract the downloaded release to a folder named `prysm-web-ui/`

4. Run the following command to generate the site_data.go file:
   ```
   go-bindata -pkg web -nometadata -modtime 0 -o site_data.go prysm-web-ui/
   ```

5. Copy and replace the generated `site_data.go` file into this directory

6. Open a PR to submit your changes

## Development

For information on developing the web UI itself, please refer to the [prysm-web-ui repository](https://github.com/prysmaticlabs/prysm-web-ui).
