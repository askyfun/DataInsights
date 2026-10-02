package model

// SystemOwnerID is the placeholder owner id recorded for rows created while the
// platform has no accounts (L0), and the value backfilled onto pre-existing rows
// by migration 00009. Zero is reserved so it never collides with a real
// bi_user.id (its autoincrement sequence starts at 1). #183 replaces this with
// the authenticated user's id taken from the request context; until then every
// create path stamps SystemOwnerID and Update never rewrites the column.
const SystemOwnerID int32 = 0
