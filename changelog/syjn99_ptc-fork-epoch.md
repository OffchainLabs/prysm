### Fixed

- `POST /eth/v1/validator/duties/ptc/{epoch}` returns 400 for the Gloas fork epoch until the fork is within `MAXIMUM_GOSSIP_CLOCK_DISPARITY`, instead of 200 with empty duties.
