# Gossipsub Control Plane Design Document

## Overview

This branch introduces a declarative, fork-aware gossipsub control plane that manages topic subscriptions and peer discovery for subnet-based topics. The system replaces ad-hoc topic management with a structured approach centered on **Topic Families**.

### Key Components

| Component | Location | Responsibility |
|-----------|----------|----------------|
| **GossipsubController** | `sync/gossipsub_controller.go` | Orchestrates topic family lifecycle across forks |
| **GossipsubPeerCrawler** | `p2p/gossipsub_peer_crawler.go` | Discovers and indexes peers by topic via discv5 |
| **GossipsubPeerDialer** | `p2p/gossipsub_peer_controller.go` | Maintains peer connections for required topics |
| **Topic Family Abstractions** | `sync/gossipsub_topic_family.go` | Interfaces for topic subscription management |

---

## 1. Topic Family Abstraction

### 1.1 Design Goals

- **Declarative Fork Management**: Topic families declare when they activate/deactivate based on fork epochs
- **Unified Subscription Logic**: Common base handles validator registration, message loops, and cleanup
- **Dynamic vs Static Distinction**: Clear separation between global topics and subnet-based topics that change per slot

### 1.2 Interface Hierarchy

```
GossipsubTopicFamily (base)
├── Name()
├── NetworkScheduleEntry()
└── UnsubscribeAll()

GossipsubTopicFamilyWithoutDynamicSubnets
└── Subscribe()  // Called once when registered

GossipsubTopicFamilyWithDynamicSubnets
├── TopicsToSubscribeForSlot(slot)
├── ExtractTopicsForNode(node)  // For peer discovery
├── SubscribeForSlot(slot)
└── UnsubscribeForSlot(slot)
```

### 1.3 Implementation Categories

**Global Topics** (subscribed once per fork):
- Block, AggregateAndProof, VoluntaryExit, ProposerSlashing, AttesterSlashing
- SyncContributionAndProof (Altair+), BlsToExecutionChange (Capella+)
- LightClient updates (Altair+, feature-flagged)

**Static Per-Subnet**:
- BlobTopicFamily - One instance per blob subnet (Deneb/Electra)

**Dynamic Subnets** (change per slot based on validator duties):
- **AttestationTopicFamily** - Subnets based on attestation committee assignments
- **SyncCommitteeTopicFamily** - Subnets based on sync committee membership
- **DataColumnTopicFamily** - Subnets based on data column custody (Fulu+)

### 1.4 Base Implementation Features

`baseGossipsubTopicFamily` provides:
- **Idempotent subscriptions** - Safe to call multiple times for same topic
- **Automatic validator registration** - Registers message validator with pubsub
- **Message loop management** - Spawns goroutine to process incoming messages
- **Cleanup coordination** - Notifies crawler when topics are unsubscribed

### 1.5 Dynamic Subnet Selection

Dynamic families combine two subnet sources:
- **Subnets to Join**: Topics we must subscribe to (persistent duties, aggregator responsibilities)
- **Subnets for Broadcast**: Topics we need peers for but may not subscribe to

| Family | Subnets to Join | Subnets for Broadcast |
|--------|-----------------|----------------------|
| Attestation | Persistent + aggregator subnets | Attester duty subnets |
| SyncCommittee | Active sync committee subnets | (none) |
| DataColumn | Custody column subnets | All column subnets |

### 1.6 Fork Schedule

Topic families declare activation and deactivation epochs (both are non-optional):

| Fork | Activations | Deactivations |
|------|-------------|---------------|
| Genesis | Block, AggregateAndProof, VoluntaryExit, ProposerSlashing, AttesterSlashing, Attestation | - |
| Altair | SyncContributionAndProof, SyncCommittee, [LightClient*] | - |
| Capella | BlsToExecutionChange | - |
| Deneb | Blob (6 subnets) | - |
| Electra | Blob (9 subnets) | Blob (Deneb config) |
| Fulu | DataColumn | Blob (all) |

---

## 2. GossipsubController

### 2.1 Responsibilities

- **Fork-Aware Topic Management**: Automatically subscribes/unsubscribes based on fork schedule
- **Smooth Fork Transitions**: Pre-subscribes 1 epoch before fork, unsubscribes 1 epoch after
- **Slot-Based Updates**: Updates dynamic subnet subscriptions every slot
- **Topic Extraction**: Provides interface for crawler to determine peer topic relevance

### 2.2 Lifecycle

1. **Startup**: Waits for initial sync to complete
2. **Control Loop**: Runs on slot ticker, calling `updateActiveTopicFamilies()`
3. **Shutdown**: Unsubscribes all families, cancels context

### 2.3 Fork Transition Handling

**Timeline for Fork at Epoch N:**
```
Epoch N-1: Subscribe to both old and new fork topics (overlap period)
Epoch N:   Fork occurs, both topic sets remain active
Epoch N+1: Unsubscribe from old fork topics, only new fork active
```

This ensures no message loss during the transition window.

### 2.4 Update Logic (per slot)

1. **Get families for current epoch** from declarative schedule
2. **Check for upcoming fork** - if next epoch is fork boundary, include next fork's families
3. **Register new families** - add to active map, subscribe based on type:
   - Static families: `Subscribe()` once
   - Dynamic families: `SubscribeForSlot()` and `UnsubscribeForSlot()` every slot
4. **Remove old fork families** - if 1 epoch past fork boundary, unsubscribe and remove

### 2.5 Topic Extraction for Peer Discovery

The controller exposes `ExtractTopics(node)` which:
- Iterates all active **dynamic** subnet families
- Calls `ExtractTopicsForNode(node)` on each
- Returns deduplicated list of topics the node can serve

This is used by the peer crawler to index discovered peers by topic.

### 2.6 Topics Provider

The controller exposes `GetCurrentActiveTopics()` which:
- Returns all topics from dynamic families for the current slot
- Used by the peer dialer to know which topics need peer connections

---

## 3. GossipsubPeerCrawler

### 3.1 Purpose

Discovers peers via discv5, indexes them by topic, and verifies reachability via ping. Provides the peer dialer with a pool of verified, scored peers for each topic.

### 3.2 Key Design Decisions

**Triple Index Structure:**
- `byEnode` - Fast lookup by enode ID
- `byPeerId` - Fast lookup by libp2p peer ID
- `byTopic` - Fast lookup of peers serving a topic

**Ping-Once Guarantee:**
- A node is pinged exactly **once** regardless of ENR sequence number updates
- Prevents ping explosion when nodes frequently update their records
- Ping success sets `isPinged=true`, failure removes peer entirely

**Sequence Number Handling:**
- Only updates peer record if new sequence number is higher
- Stale records are ignored to prevent processing outdated data

### 3.3 Three Concurrent Loops

| Loop | Interval | Purpose |
|------|----------|---------|
| **crawlLoop** | `crawlInterval` | Iterates discv5 `RandomNodes()`, extracts topics, updates index |
| **pingLoop** | Continuous | Consumes ping queue, verifies reachability |
| **cleanupLoop** | 5 minutes | Prunes peers that fail filter or have no relevant topics |

### 3.4 Crawl Flow

1. Create timeout context for crawl iteration
2. Get random nodes iterator from discv5
3. For each node:
   - Apply peer filter (reject bad/incompatible peers)
   - Extract topics via `topicExtractor` (provided by controller)
   - Update index if sequence number is newer
   - Queue for ping if not already pinged and has topics

### 3.5 Ping Queue and Backpressure

- **Channel capacity**: `4 * maxConcurrentPings`
- **Backpressure**: When queue is full, crawl loop blocks on send
- **Semaphore**: Limits concurrent ping goroutines to `maxConcurrentPings`
- **Ping failure**: Removes peer from index entirely (unreachable)
- **Ping success**: Marks peer as verified (`isPinged=true`)

### 3.6 Peer Retrieval (`PeersForTopic`)

Returns peers for a topic with guarantees:
1. **Only pinged peers** - Verified reachable
2. **Filter applied** - Passes current peer filter
3. **Sorted by score** - Best peers first (using p2p scorer)

### 3.7 Peer Removal Triggers

| Trigger | Behavior |
|---------|----------|
| Ping failure | Remove immediately |
| Peer disconnection | `RemovePeerId()` called from disconnect handler |
| Topic unsubscription | `RemoveTopic()` called from base family cleanup |
| Filter rejection during crawl | Remove if previously indexed |
| Cleanup loop | Remove if no longer passes filter or has no topics |

### 3.8 Topic Extraction for Dynamic Subnets

For each dynamic family, extraction:
1. Gets subnets we currently need (union of join + broadcast)
2. Reads subnet bitfield from node's ENR record
3. Returns intersection - topics both we need AND the node advertises

---

## 4. GossipsubPeerDialer

### 4.1 Purpose

Maintains peer connections for topics we need. Works with the crawler to dial verified peers when topic peer counts fall below threshold.

### 4.2 Key Design Decisions

**Target Peer Count**: 20 peers per topic (`peerPerTopic` constant)

**Dial Loop Frequency**: Every 1 second

**Deduplication**: Peers appearing for multiple topics are only dialed once

### 4.3 Dial Flow

1. Get current topics from `topicsProvider` (controller's `GetCurrentActiveTopics`)
2. For each topic:
   - Check current connected peer count via `listPeersFunc`
   - If below target, calculate how many more needed
   - Get peers from crawler (already filtered, scored, pinged)
   - Limit to what's needed
3. Deduplicate peer list across all topics
4. Dial peers via `dialPeersFunc`

### 4.4 Blocking Dial

`DialPeersForTopicBlocking(ctx, topic, nPeers)` provides synchronous peer acquisition:
- Loops until target peer count reached or context cancelled
- Used for critical operations that need guaranteed peer connectivity
- Polls every 100ms to check connection status

---

## 5. Component Interactions

### 5.1 Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Sync Service                                    │
│  ┌───────────────────────────────────────────────────────────────────────┐  │
│  │                      GossipsubController                               │  │
│  │                                                                        │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐       │  │
│  │  │ AttestationTF   │  │ SyncCommitteeTF │  │ DataColumnTF    │       │  │
│  │  │ (dynamic)       │  │ (dynamic)       │  │ (dynamic)       │       │  │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘       │  │
│  │                                                                        │  │
│  │  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐       │  │
│  │  │ BlockTF, etc.   │  │ BlobTF (static) │  │ baseTopicFamily │       │  │
│  │  │ (global)        │  │                 │  │ (shared logic)  │       │  │
│  │  └─────────────────┘  └─────────────────┘  └─────────────────┘       │  │
│  │                                                                        │  │
│  └──────────────────┬─────────────────────────────┬──────────────────────┘  │
│                     │                             │                         │
│    GetCurrentActiveTopics()               ExtractTopics()                   │
│                     │                             │                         │
└─────────────────────┼─────────────────────────────┼─────────────────────────┘
                      │                             │
                      ▼                             ▼
┌─────────────────────────────────┐  ┌─────────────────────────────────────┐
│     GossipsubPeerDialer         │  │       GossipsubPeerCrawler          │
│                                 │  │                                     │
│  - Polls topics every 1 second  │  │  - Crawls discv5 periodically       │
│  - Checks peer count per topic  │  │  - Indexes peers by topic           │
│  - Dials missing peers          │  │  - Verifies via ping                │
│                                 │  │  - Filters and scores peers         │
│         │                       │  │                                     │
│         │   PeersForTopic()     │  │         │                           │
│         └───────────────────────┼──┼─────────┘                           │
│                                 │  │                                     │
└─────────────────────────────────┘  └──────────────────┬──────────────────┘
                                                        │
                                                        │ RemovePeerId()
                                     ┌──────────────────┘
                                     │
                                     ▼
                     ┌─────────────────────────────────┐
                     │         P2P Service             │
                     │                                 │
                     │  - Disconnect handler calls     │
                     │    RemovePeerId() on crawler    │
                     │  - Provides filterPeer, scorer  │
                     └─────────────────────────────────┘
```

### 5.2 Data Flow Summary

| Flow | Description |
|------|-------------|
| **Discovery** | discv5 → crawlLoop → topicExtractor → crawledPeers index → pingCh |
| **Ping** | pingCh → semaphore → dv5.Ping() → isPinged=true or remove |
| **Dial** | controller topics → dialer → crawler.PeersForTopic() → dialPeers |
| **Cleanup** | disconnect/unsubscribe → RemovePeerId()/RemoveTopic() |

### 5.3 Key Invariants

**Peers from `PeersForTopic()` are always:**
- Successfully pinged (reachable)
- Passing the peer filter
- Sorted by score (best first)

**Topic subscriptions are:**
- Pre-subscribed 1 epoch before fork
- Unsubscribed 1 epoch after fork
- Updated every slot for dynamic families

**Ping behavior:**
- Each node ID pinged at most once
- Ping failures remove peer entirely
- Sequence number updates don't trigger re-ping

**Backpressure:**
- Ping queue blocks crawl when full
- Semaphore limits concurrent pings
- Natural rate limiting without explicit throttling

---

## 6. Initialization Sequence

```
PHASE 1: P2P Service Start
══════════════════════════
  ├─► Start discv5 listener
  ├─► Create GossipsubPeerCrawler (with filterPeer, scorer)
  └─► Create GossipsubPeerDialer (not started yet)

PHASE 2: Sync Service Start
═══════════════════════════
  ├─► Create GossipsubController
  └─► Launch startDiscoveryAndSubscriptions goroutine

PHASE 3: Discovery and Subscriptions (after chain start)
════════════════════════════════════════════════════════
  ├─► Start GossipsubController (control loop)
  ├─► Start Crawler with topicExtractor from controller
  └─► Start Dialer with topicsProvider from controller
```

### Dependency Injection

| Component | Dependencies | Provider |
|-----------|-------------|----------|
| Crawler | discv5, filterPeer, scorer | P2P Service |
| Crawler | topicExtractor | GossipsubController |
| Dialer | crawler, listPeers, dialPeers | P2P Service |
| Dialer | topicsProvider | GossipsubController |

---

## 7. Configuration Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `crawlInterval` | configurable | How often to crawl discv5 |
| `crawlTimeout` | configurable | Max duration per crawl iteration |
| `maxConcurrentPings` | configurable | Parallel ping limit |
| `cleanupInterval` | 5 minutes | Stale peer pruning frequency |
| `peerPerTopic` | 20 | Target peer count per topic |
| `dialLoop interval` | 1 second | Topic peer check frequency |

---

## 8. Key Files

| File | Purpose |
|------|---------|
| `sync/gossipsub_controller.go` | Controller orchestrating topic families |
| `sync/gossipsub_topic_family.go` | Interface definitions and fork schedule |
| `sync/gossipsub_base.go` | Base implementation for all topic families |
| `sync/topic_families_without_subnets.go` | Global topic family implementations |
| `sync/topic_families_static_subnets.go` | Blob topic family |
| `sync/topic_families_dynamic_subnets.go` | Dynamic subnet families |
| `p2p/gossipsub_peer_crawler.go` | Peer discovery and indexing |
| `p2p/gossipsub_peer_controller.go` | Peer dialing logic |
| `p2p/gossipsubcrawler/interface.go` | Shared interfaces |
| `p2p/handshake.go` | Disconnect handler integration |
