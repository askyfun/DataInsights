package queryrecord

import "errors"

// ErrNotFound is returned by GetByID when no record with the given id exists.
// A soft-expired record is NOT a "not found": it is still returned by GetByID.
var ErrNotFound = errors.New("query record not found")

// ErrSpecTooLarge is returned by Record when the canonical spec_json exceeds the
// configured byte limit. The caller must surface this to the user instead of
// silently truncating the record.
var ErrSpecTooLarge = errors.New("spec_json exceeds size limit")
