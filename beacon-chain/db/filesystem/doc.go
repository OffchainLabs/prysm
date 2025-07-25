package filesystem

// nolint:dupword
/*
Data column sidecars storage documentation
==========================================

File organisation
-----------------
- The first byte represents the version of the file structure (up to 0xff = 255).
  We set it to 0x01.
  Note: This is not strictly needed, but it will help a lot if, in the future,
        we want to modify the file structure.
- The next 4 bytes represents the size of a SSZ encoded data column sidecar.
  (See the `Computation of the maximum size of a DataColumnSidecar` section to a description
  of how this value is computed).
- The next 128 bytes represent the index in the file of a given column.
  The first bit of each byte in the index is set to 0 if there is no data column,
  and set to 1 if there is a data column.
  The remaining 7 bits (from 0 to 127) represent the index of the data column.
  This sentinel bit is needed to distinguish between the column with index 0 and no column.
  Example: If the column with index 5 is in the 3th position in the file, then indices[5] = 0x80 + 0x03 = 0x83.
- The rest of the file is a repeat of the SSZ encoded data column sidecars.


|------------------------------------------|------------------------------------------------------------------------------------|
|                Byte offset               |                                     Description                                    |
|------------------------------------------|------------------------------------------------------------------------------------|
|                                        0 | version (1 byte) | sszEncodedDataColumnSidecarSize (4 bytes) | indices (128 bytes) |
|133 +   0*sszEncodedDataColumnSidecarSize | sszEncodedDataColumnSidecar (sszEncodedDataColumnSidecarSize bytes)                |
|133 +   1*sszEncodedDataColumnSidecarSize | sszEncodedDataColumnSidecar (sszEncodedDataColumnSidecarSize bytes)                |
|133 +   2*sszEncodedDataColumnSidecarSize | sszEncodedDataColumnSidecar (sszEncodedDataColumnSidecarSize bytes)                |
|                  ...                     |                                 ...                                                |
|133 + 127*sszEncodedDataColumnSidecarSize | sszEncodedDataColumnSidecar (sszEncodedDataColumnSidecarSize bytes)                |
|------------------------------------------|------------------------------------------------------------------------------------|

Each file is named after the block root where the data columns were data columns are committed to.
Example: `0x259c6d2f6a0bb75e2405cea7cb248e5663dc26b9404fd3bcd777afc20de91c1e.sszs`

Database organisation
---------------------
SSZ encoded data column sidecars are stored following the `by-epoch` layout.
- The first layer is a directory corresponding to the `period`, which corresponds to the epoch divided by the 4096.
- The second layer is a directory corresponding to the epoch.
- Then all files are stored in the epoch directory.

Example:
data-columns
├── 0
│   ├── 3638
│   │   ├── 0x259c6d2f6a0bb75e2405cea7cb248e5663dc26b9404fd3bcd777afc20de91c1e.sszs
│   │   ├── 0x2a855b1f6e9a2f04f8383e336325bf7d5ba02d1eab3ef90ef183736f8c768533.sszs
│   │   ├── ...
│   │   ├── 0xeb78e2b2350a71c640f1e96fea9e42f38e65705ab7e6e100c8bc9c589f2c5f2b.sszs
│   │   └── 0xeb7ee68da988fd20d773d45aad01dd62527734367a146e2b048715bd68a4e370.sszs
│   └── 3639
│       ├── 0x0fd231fe95e57936fa44f6c712c490b9e337a481b661dfd46768901e90444330.sszs
│       ├── 0x1bf5edff6b6ba2b65b1db325ff3312bbb57da461ef2ae651bd741af851aada3a.sszs
│       ├── ...
│       ├── 0xa156a527e631f858fee79fab7ef1fde3f6117a2e1201d47c09fbab0c6780c937.sszs
│       └── 0xcd80bc535ddc467dea1d19e0c39c1160875ccd1989061bcd8ce206e3c1261c87.sszs
└── 1
    ├── 4096
    │   ├── 0x0d244009093e2bedb72eb265280290199e8c7bf1d90d7583c41af40d9f662269.sszs
    │   ├── 0x11f420928d8de41c50e735caab0369996824a5299c5f054e097965855925697d.sszs
    │   ├── ...
    │   ├── 0xbe91fc782877ed400d95c02c61aebfdd592635d11f8e64c94b46abd84f45c967.sszs
    │   └── 0xf246189f078f02d30173ff74605cf31c9e65b5e463275ebdbeb40476638135ff.sszs
    └── 4097
        ├── 0x454d000674793c479e90504c0fe9827b50bb176ae022dab4e37d6a21471ab570.sszs
        ├── 0xac5eb7437d7190c48cfa863e3c45f96a7f8af371d47ac12ccda07129a06af763.sszs
        ├── ...
        ├── 0xb7df30561d9d92ab5fafdd96bca8b44526497c8debf0fc425c7a0770b2abeb83.sszs
        └── 0xc1dd0b1ae847b6ec62303a36d08c6a4a2e9e3ec4be3ff70551972a0ee3de9c14.sszs

Computation of the maximum size of a DataColumnSidecar
------------------------------------------------------
https://github.com/ethereum/consensus-specs/blob/master/specs/fulu/das-core.md#datacolumnsidecar


class DataColumnSidecar(Container):
    index: ColumnIndex  # Index of column in extended matrix
    column: List[Cell, MAX_BLOB_COMMITMENTS_PER_BLOCK]
    kzg_commitments: List[KZGCommitment, MAX_BLOB_COMMITMENTS_PER_BLOCK]
    kzg_proofs: List[KZGProof, MAX_BLOB_COMMITMENTS_PER_BLOCK]
    signed_block_header: SignedBeaconBlockHeader
    kzg_commitments_inclusion_proof: Vector[Bytes32, KZG_COMMITMENTS_INCLUSION_PROOF_DEPTH]


- index: 2 bytes (ColumnIndex)
- `column`: 4,096 (MAX_BLOB_COMMITMENTS_PER_BLOCK) * 64 (FIELD_ELEMENTS_PER_CELL) * 32 bytes (BYTES_PER_FIELD_ELEMENT) = 8,388,608 bytes
- kzg_commitments: 4,096 (MAX_BLOB_COMMITMENTS_PER_BLOCK) * 48 bytes (KZGCommitment) = 196,608 bytes
- kzg_proofs: 4,096 (MAX_BLOB_COMMITMENTS_PER_BLOCK) * 48 bytes (KZGProof) = 196,608 bytes
- signed_block_header: 2 bytes (Slot) + 2 bytes (ValidatorIndex) + 3 * 2 bytes (Root) + 96 bytes (BLSSignature) = 106 bytes
- kzg_commitments_inclusion_proof: 4 (KZG_COMMITMENTS_INCLUSION_PROOF_DEPTH) * 32 bytes = 128 bytes

TOTAL: 8,782,060 bytes = 70,256,480 bits
log(70,256,480) / log(2) ~= 26.07

==> 32 bits (4 bytes) are enough to store the maximum size of a data column sidecar.

The maximum size of an SSZ encoded data column can be 2**32 bits = 536,879,912 bytes,
which left a room of 536,879,912 bytes - 8,782,060 bytes ~= 503 mega bytes to store the extra data needed by SSZ encoding (which is more than enough.)
*/
