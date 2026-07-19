// IC status types (GetBy / PutBy / InBy / DeleteBy / InstanceOf)
package bytecode

type GetByStatus struct{ state uint8 }
type GetByVariant struct{}
type PutByStatus struct{ state uint8 }
type PutByVariant struct{}
type InByStatus struct{ state uint8 }
type InByVariant struct{}
type DeleteByStatus struct{ state uint8 }
type DeleteByVariant struct{}
type InstanceOfStatus struct{ state uint8 }
type InstanceOfVariant struct{}
type ICStatusMap struct{}
type ICStatusUtils struct{}
type RecordedStatuses struct{}
