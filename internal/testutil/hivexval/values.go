package hivexval

import (
	"errors"
	"fmt"
	"time"

	"github.com/joshuapare/hivekit/bindings"
	"github.com/joshuapare/hivekit/pkg/types"
)

// GetValues lists all values in a key.
//
// Example:
//
//	values, err := v.GetValues(key)
//	for _, val := range values {
//	    name, _ := v.GetValueName(val)
//	    t.Logf("Value: %s", name)
//	}
func (v *Validator) GetValues(key interface{}) ([]interface{}, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := key.(bindings.NodeHandle)
		if !ok {
			return nil, errors.New("invalid key handle type for bindings")
		}
		values := v.hive.NodeValues(node)
		result := make([]interface{}, len(values))
		for i, val := range values {
			result[i] = val
		}
		return result, nil

	case BackendReader:
		nodeID, ok := key.(types.NodeID)
		if !ok {
			return nil, errors.New("invalid key handle type for reader")
		}
		values, err := v.reader.Values(nodeID)
		if err != nil {
			return nil, err
		}
		result := make([]interface{}, len(values))
		for i, val := range values {
			result[i] = val
		}
		return result, nil

	case BackendNone, BackendHivexsh:
		return nil, errors.New("backend does not support GetValues")

	default:
		return nil, errors.New("backend does not support GetValues")
	}
}

// GetValue finds a value by name (case-insensitive).
//
// Example:
//
//	val, err := v.GetValue(key, "Version")
//	if err != nil {
//	    t.Fatal("Value not found")
//	}
func (v *Validator) GetValue(key interface{}, name string) (interface{}, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := key.(bindings.NodeHandle)
		if !ok {
			return nil, errors.New("invalid key handle type for bindings")
		}
		val := v.hive.NodeGetValue(node, name)
		if val == 0 {
			return nil, fmt.Errorf("value '%s' not found", name)
		}
		return val, nil

	case BackendReader:
		nodeID, ok := key.(types.NodeID)
		if !ok {
			return nil, errors.New("invalid key handle type for reader")
		}
		return v.reader.GetValue(nodeID, name)

	case BackendNone, BackendHivexsh:
		return nil, errors.New("backend does not support GetValue")

	default:
		return nil, errors.New("backend does not support GetValue")
	}
}

// GetValueCount returns number of values in a key.
func (v *Validator) GetValueCount(key interface{}) (int, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := key.(bindings.NodeHandle)
		if !ok {
			return 0, errors.New("invalid key handle type for bindings")
		}
		return v.hive.NodeNrValues(node), nil

	case BackendReader:
		nodeID, ok := key.(types.NodeID)
		if !ok {
			return 0, errors.New("invalid key handle type for reader")
		}
		return v.reader.KeyValueCount(nodeID)

	case BackendNone, BackendHivexsh:
		return 0, errors.New("backend does not support GetValueCount")

	default:
		return 0, errors.New("backend does not support GetValueCount")
	}
}

// GetValueName returns value name.
func (v *Validator) GetValueName(val interface{}) (string, error) {
	switch v.backend {
	case BackendBindings:
		valHandle, ok := val.(bindings.ValueHandle)
		if !ok {
			return "", errors.New("invalid value handle type for bindings")
		}
		return v.hive.ValueKey(valHandle), nil

	case BackendReader:
		valID, ok := val.(types.ValueID)
		if !ok {
			return "", errors.New("invalid value handle type for reader")
		}
		return v.reader.ValueName(valID)

	case BackendNone, BackendHivexsh:
		return "", errors.New("backend does not support GetValueName")

	default:
		return "", errors.New("backend does not support GetValueName")
	}
}

// GetValueType returns value type as string.
//
// Returns type names like "REG_SZ", "REG_DWORD", "REG_BINARY", etc.
//
// Example:
//
//	typ, _ := v.GetValueType(val)
//	require.Equal(t, "REG_SZ", typ)
func (v *Validator) GetValueType(val interface{}) (string, error) {
	switch v.backend {
	case BackendBindings:
		valHandle, ok := val.(bindings.ValueHandle)
		if !ok {
			return "", errors.New("invalid value handle type for bindings")
		}
		vt, _, err := v.hive.ValueType(valHandle)
		if err != nil {
			return "", err
		}
		return valueTypeToString(int(vt)), nil

	case BackendReader:
		valID, ok := val.(types.ValueID)
		if !ok {
			return "", errors.New("invalid value handle type for reader")
		}
		regType, err := v.reader.ValueType(valID)
		if err != nil {
			return "", err
		}
		return valueTypeToString(int(regType)), nil

	case BackendNone, BackendHivexsh:
		return "", errors.New("backend does not support GetValueType")

	default:
		return "", errors.New("backend does not support GetValueType")
	}
}

// GetValueData returns raw value bytes.
func (v *Validator) GetValueData(val interface{}) ([]byte, error) {
	switch v.backend {
	case BackendBindings:
		valHandle, ok := val.(bindings.ValueHandle)
		if !ok {
			return nil, errors.New("invalid value handle type for bindings")
		}
		data, _, err := v.hive.ValueValue(valHandle)
		return data, err

	case BackendReader:
		valID, ok := val.(types.ValueID)
		if !ok {
			return nil, errors.New("invalid value handle type for reader")
		}
		return v.reader.ValueBytes(valID, types.ReadOptions{})

	case BackendNone, BackendHivexsh:
		return nil, errors.New("backend does not support GetValueData")

	default:
		return nil, errors.New("backend does not support GetValueData")
	}
}

// GetValueString returns value as string (REG_SZ/REG_EXPAND_SZ).
//
// Example:
//
//	str, err := v.GetValueString(val)
//	require.NoError(t, err)
//	require.Equal(t, "1.0.0", str)
func (v *Validator) GetValueString(val interface{}) (string, error) {
	switch v.backend {
	case BackendBindings:
		valHandle, ok := val.(bindings.ValueHandle)
		if !ok {
			return "", errors.New("invalid value handle type for bindings")
		}
		return v.hive.ValueString(valHandle)

	case BackendReader:
		valID, ok := val.(types.ValueID)
		if !ok {
			return "", errors.New("invalid value handle type for reader")
		}
		return v.reader.ValueString(valID, types.ReadOptions{})

	case BackendNone, BackendHivexsh:
		return "", errors.New("backend does not support GetValueString")

	default:
		return "", errors.New("backend does not support GetValueString")
	}
}

// GetValueDWORD returns value as uint32 (REG_DWORD).
func (v *Validator) GetValueDWORD(val interface{}) (uint32, error) {
	switch v.backend {
	case BackendBindings:
		valHandle, ok := val.(bindings.ValueHandle)
		if !ok {
			return 0, errors.New("invalid value handle type for bindings")
		}
		dw, err := v.hive.ValueDword(valHandle)
		if err != nil {
			return 0, err
		}
		return uint32(dw), nil

	case BackendReader:
		valID, ok := val.(types.ValueID)
		if !ok {
			return 0, errors.New("invalid value handle type for reader")
		}
		return v.reader.ValueDWORD(valID)

	case BackendNone, BackendHivexsh:
		return 0, errors.New("backend does not support GetValueDWORD")

	default:
		return 0, errors.New("backend does not support GetValueDWORD")
	}
}

// GetValueQWORD returns value as uint64 (REG_QWORD).
func (v *Validator) GetValueQWORD(val interface{}) (uint64, error) {
	switch v.backend {
	case BackendBindings:
		valHandle, ok := val.(bindings.ValueHandle)
		if !ok {
			return 0, errors.New("invalid value handle type for bindings")
		}
		qw, err := v.hive.ValueQword(valHandle)
		if err != nil {
			return 0, err
		}
		return uint64(qw), nil

	case BackendReader:
		valID, ok := val.(types.ValueID)
		if !ok {
			return 0, errors.New("invalid value handle type for reader")
		}
		return v.reader.ValueQWORD(valID)

	case BackendNone, BackendHivexsh:
		return 0, errors.New("backend does not support GetValueQWORD")

	default:
		return 0, errors.New("backend does not support GetValueQWORD")
	}
}

// GetValueStrings returns value as string slice (REG_MULTI_SZ).
//
// Example:
//
//	strs, err := v.GetValueStrings(val)
//	require.NoError(t, err)
//	require.Equal(t, []string{"A", "B", "C"}, strs)
func (v *Validator) GetValueStrings(val interface{}) ([]string, error) {
	switch v.backend {
	case BackendBindings:
		valHandle, ok := val.(bindings.ValueHandle)
		if !ok {
			return nil, errors.New("invalid value handle type for bindings")
		}
		return v.hive.ValueMultipleStrings(valHandle)

	case BackendReader:
		valID, ok := val.(types.ValueID)
		if !ok {
			return nil, errors.New("invalid value handle type for reader")
		}
		return v.reader.ValueStrings(valID, types.ReadOptions{})

	case BackendNone, BackendHivexsh:
		return nil, errors.New("backend does not support GetValueStrings")

	default:
		return nil, errors.New("backend does not support GetValueStrings")
	}
}

// GetKeyTimestamp returns last write time.
func (v *Validator) GetKeyTimestamp(key interface{}) (time.Time, error) {
	switch v.backend {
	case BackendBindings:
		node, ok := key.(bindings.NodeHandle)
		if !ok {
			return time.Time{}, errors.New("invalid key handle type for bindings")
		}
		ts := v.hive.NodeTimestamp(node)
		// Convert from Windows FILETIME to Unix time
		return time.Unix(0, ts*100), nil

	case BackendReader:
		nodeID, ok := key.(types.NodeID)
		if !ok {
			return time.Time{}, errors.New("invalid key handle type for reader")
		}
		return v.reader.KeyTimestamp(nodeID)

	case BackendNone, BackendHivexsh:
		return time.Time{}, errors.New("backend does not support GetKeyTimestamp")

	default:
		return time.Time{}, errors.New("backend does not support GetKeyTimestamp")
	}
}

// valueTypeToString converts registry type constant to string name.
func valueTypeToString(regType int) string {
	switch regType {
	case 0:
		return "REG_NONE"
	case 1:
		return "REG_SZ"
	case 2:
		return "REG_EXPAND_SZ"
	case 3:
		return "REG_BINARY"
	case 4:
		return "REG_DWORD"
	case 5:
		return "REG_DWORD_BIG_ENDIAN"
	case 6:
		return "REG_LINK"
	case 7:
		return "REG_MULTI_SZ"
	case 8:
		return "REG_RESOURCE_LIST"
	case 9:
		return "REG_FULL_RESOURCE_DESCRIPTOR"
	case 10:
		return "REG_RESOURCE_REQUIREMENTS_LIST"
	case 11:
		return "REG_QWORD"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", regType)
	}
}
