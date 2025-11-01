## Changed
- Introduced Path type for SSZ-QL queries and updated PathElement (removed Length field, kept Index) enforcing that len queries are terminal (at most one per path).
- Changed length query syntax from `block.payload.len(transactions)` to `len(block.payload.transactions)`
