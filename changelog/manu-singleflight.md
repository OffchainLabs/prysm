### Fixed 
- `RODataColumnsVerifier.ValidProposerSignature`: Ensure the expensive signature verification is only performed once for concurrent requests for the same signature data.