package decorator

import (
	"context"

	"github.com/BayCEER/bayeos/bayeos-api/model"
	"github.com/BayCEER/bayeos/bayeos-api/service"
)

// stubNode is a service.NodeService that returns canned results and records the
// calls it receives. It embeds the interface so the tests only need to implement
// the methods they exercise, which is the practical benefit of decorating an
// interface rather than a concrete type.
type stubNode struct {
	service.NodeService

	node  model.Node
	nodes []model.Node
	err   error

	// total is returned by FindNodesByAntPath.
	total int32

	// calls records the method name of every call, in order.
	calls []string

	// fields records the last argument of UpdateNode.
	lastFields map[string]any

	// lastDeleteIDs records the argument of DeleteNodesByIDs.
	lastDeleteIDs []int32
}

func (s *stubNode) GetNode(_ context.Context, _ int32, _ int32) (model.Node, error) {
	s.calls = append(s.calls, "GetNode")
	return s.node, s.err
}

func (s *stubNode) GetNodes(_ context.Context, _ []int32, _ int32) ([]model.Node, error) {
	s.calls = append(s.calls, "GetNodes")
	return s.nodes, s.err
}

func (s *stubNode) GetNodeChildren(_ context.Context, _ int32, _ int32) ([]model.Node, error) {
	s.calls = append(s.calls, "GetNodeChildren")
	return s.nodes, s.err
}

func (s *stubNode) GetNodeParents(_ context.Context, _ int32, _ int32) ([]model.Node, error) {
	s.calls = append(s.calls, "GetNodeParents")
	return s.nodes, s.err
}

func (s *stubNode) FindNodesByAntPath(_ context.Context, _ string, _ string, _ int, _ int, _ int32) (int32, []model.Node, error) {
	s.calls = append(s.calls, "FindNodesByAntPath")
	return s.total, s.nodes, s.err
}



func (s *stubNode) CreateNode(_ context.Context, node *model.Node, _ int32) error {
	s.calls = append(s.calls, "CreateNode")
	if s.err != nil {
		return s.err
	}
	if node != nil {
		s.node = *node
	}
	return nil
}

func (s *stubNode) CreateNodeByPath(_ context.Context, _ *model.Node, _ int32, _ string, _ int32) error {
	s.calls = append(s.calls, "CreateNodeByPath")
	return s.err
}

func (s *stubNode) UpdateNode(_ context.Context, _ int32, fields *map[string]any) error {
	s.calls = append(s.calls, "UpdateNode")
	if fields != nil {
		s.lastFields = *fields
	}
	return s.err
}

func (s *stubNode) UpdateNodeMetadata(_ context.Context, _ int32, _ map[string]any) error {
	s.calls = append(s.calls, "UpdateNodeMetadata")
	return s.err
}

func (s *stubNode) DeleteNode(_ context.Context, _ int32) error {
	s.calls = append(s.calls, "DeleteNode")
	return s.err
}

func (s *stubNode) DeleteNodesByIDs(_ context.Context, ids []int32) error {
	s.calls = append(s.calls, "DeleteNodesByIDs")
	s.lastDeleteIDs = ids
	return s.err
}

// newNode returns a node with the given id, kind and metadata.
func newNode(id int32, kind string, md map[string]any) model.Node {
	metadata, err := model.MetadataFromMap(md)
	if err != nil {
		panic(err)
	}
	return model.Node{
		ID:          id,
		Kind:        kind,
		DisplayName: "node",
		Metadata:    metadata,
	}
}

// mdOf returns the node metadata as a map, for assertions.
func mdOf(n model.Node) map[string]any {
	return n.Metadata.Map()
}
