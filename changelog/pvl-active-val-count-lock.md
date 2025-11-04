### Fixed

- Changed the behavior of topic subscriptions such that only topics that require the active validator count will compute that value. 
- Added a Mutex to the computation of active validator count during topic subscription to avoid a race condition where multiple goroutines are computing the same work.
