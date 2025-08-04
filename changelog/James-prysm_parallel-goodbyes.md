### Changed

- when shutting down the sync service we now send p2p goodbye messages in parallel to maxmimize changes of propogating goodbyes to all peers before an unsafe shutdown. 