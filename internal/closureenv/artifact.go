package closureenv

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const ArtifactVersion = 1

type Artifact struct {
	Version int    `json:"version"`
	Growth  Growth `json:"growth"`
}

func WriteArtifact(writer io.Writer, growth *Growth) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(Artifact{Version: ArtifactVersion, Growth: *growth})
}

func ReadArtifact(reader io.Reader) (*Growth, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var artifact Artifact
	if err := decoder.Decode(&artifact); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("trailing closure evidence value")
		}
		return nil, err
	}
	if artifact.Version != ArtifactVersion {
		return nil, fmt.Errorf("unsupported closure evidence version %d", artifact.Version)
	}
	return Compare(&artifact.Growth.Before, &artifact.Growth.After)
}
