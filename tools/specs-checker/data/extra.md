```python
def Sign(SK: int, message: Bytes) -> BLSSignature
```
```python
def Verify(PK: BLSPubkey, message: Bytes, signature: BLSSignature) -> bool
```
```python
def AggregateVerify(pairs: Sequence[PK: BLSPubkey, message: Bytes], signature: BLSSignature) -> bool
```
```python
def FastAggregateVerify(PKs: Sequence[BLSPubkey], message: Bytes, signature: BLSSignature) -> bool
```
```python
def Aggregate(signatures: Sequence[BLSSignature]) -> BLSSignature
```
