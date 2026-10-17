# Keymanager API

https://github.com/ethereum/keymanager-APIs

## Postman

You can use Postman to test the API. https://www.postman.com/

### Postman collection

In this package you will find the Postman collection for the keymanager API. 
You can import this collection in your own Postman instance to test the API.

#### Updating the collection

The collection will need to be exported and overwritten to update the collection. A PR should be created once the file
is updated.

#### Authentication

Our keymanager API requires a valid bearer token to run the keymanager. 
The token is written to the path given by `--keymanager-token-file` when running `validator --rpc`, and can also be
(re)generated with `validator generate-auth-token`.
The token can be copied and pasted into the authorization tab of each Postman request to authenticate.

