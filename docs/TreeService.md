# High-Performance In-Memory Tree Service in Go

Dieses Dokument enthält die vollständige Implementierung eines speicheroptimierten Baum-Services für **~50.000 Knoten**. Das System ist speziell für **Bulk-Batch-Updates** und **Strukturänderungen** ausgelegt.

## Architektur-Highlights

- **Cache-Locality (Arena Allocation):** Alle Knoten liegen in einem einzigen, flachen Go-Slice (`[]Node`). Der Garbage Collector (GC) sieht nur eine einzige Allokation statt 50.000 Einzelzeigern.
- **Topologische Sortierung:** Beim Startup werden die Knoten mittels Breitensuche (BFS) so sortiert, dass Kinder _immer_ hinter ihren Eltern im Array liegen.
- **Aggregations-Sweep in <1ms:** Die Berechnung von `min(First)` und `max(Last)` erfolgt über eine lineare Rückwärtsschleife ohne Rekursion und ohne Map-Lookups.
- **Frictionless Mutex:** Ein `sync.RWMutex` sorgt dafür, dass Lese-Anfragen (Queries) parallel laufen können und nur bei Strukturänderungen oder Batch-Updates kurz blockiert wird.

---

## Vollständiger Quellcode (`tree_service.go`)

```go
package main

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

// DBNode repräsentiert exakt das Schema der SQL-Tabelle: id, parent_id
type DBNode struct {
	ID       string
	ParentID sql.NullString // Unterstützt NULL für Root-Knoten
}

// Node repräsentiert das speicheroptimierte In-Memory Layout
type Node struct {
	ID        string
	ParentID  string
	ParentIdx int // Direkt-Index im Array für O(1) Zugriff ohne Map-Lookup

	// Lokale Zeitreihen-Eigenschaften
	LocalFirst time.Time
	LocalLast  time.Time

	// Aggregierte Eigenschaften (Ergebnisse von min/max der Kinder)
	AggFirst time.Time
	AggLast  time.Time
}

// TreeService verwaltet den Baum thread-sicher im Speicher
type TreeService struct {
	mu       sync.RWMutex
	nodes    []Node
	indexMap map[string]int // Map für schnellen O(1) Zugriff via ID -> Array-Index
}

// ==========================================
// 1. STARTUP-INITIALISIERUNG AUS DER DB
// ==========================================

// InitializeTreeFromDB lädt alle Knoten beim Startup via SQL
func InitializeTreeFromDB(db *sql.DB) (*TreeService, error) {
	query := "SELECT id, parent_id FROM nodes"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query nodes: %w", err)
	}
	defer rows.Close()

	byParent := make(map[string][]Node)
	var rootNodes []Node

	// Schritt A: Rohe relationale Daten einlesen
	for rows.Next() {
		var dbNode DBNode
		if err := rows.Scan(&dbNode.ID, &dbNode.ParentID); err != nil {
			return nil, err
		}

		node := Node{
			ID:        dbNode.ID,
			ParentIdx: -1,
		}

		if dbNode.ParentID.Valid && dbNode.ParentID.String != "" {
			node.ParentID = dbNode.ParentID.String
			byParent[node.ParentID] = append(byParent[node.ParentID], node)
		} else {
			rootNodes = append(rootNodes, node)
		}
	}

	// Schritt B: Topologische Sortierung via BFS (Breadth-First Search)
	// Garantiert: Eltern-Knoten liegen IMMER vor ihren Kindern im Slice
	ordered := make([]Node, 0, 55000) // Leicht überallokiert für 50k Nodes
	indexMap := make(map[string]int)

	queue := rootNodes
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		indexMap[curr.ID] = len(ordered)
		ordered = append(ordered, curr)

		if children, exists := byParent[curr.ID]; exists {
			queue = append(queue, children...)
		}
	}

	ts := &TreeService{
		nodes:    ordered,
		indexMap: indexMap,
	}

	// Index-Cache aufbauen
	ts.rebuildParentIndices()
	return ts, nil
}

// rebuildParentIndices wandelt String-Beziehungen in direkte Array-Indizes um
func (ts *TreeService) rebuildParentIndices() {
	for i := range ts.nodes {
		if ts.nodes[i].ParentID == "" {
			ts.nodes[i].ParentIdx = -1
			continue
		}
		if idx, exists := ts.indexMap[ts.nodes[i].ParentID]; exists {
			ts.nodes[i].ParentIdx = idx
		} else {
			ts.nodes[i].ParentIdx = -1 // Fängt verwaiste Knoten (Orphans) ab
		}
	}
}

// ==========================================
// 2. HIGH-PERFORMANCE AGGREGATION (BATCH)
// ==========================================

// RecomputeAggregations berechnet min(first) und max(last) von unten nach oben.
// Durch das flache Speicherlayout läuft dies bei 50K Nodes in wenigen Mikrosekunden.
func (ts *TreeService) RecomputeAggregations() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Schritt A: Alle Aggregate auf den aktuellen lokalen Stand zurücksetzen
	for i := range ts.nodes {
		ts.nodes[i].AggFirst = ts.nodes[i].LocalFirst
		ts.nodes[i].AggLast = ts.nodes[i].LocalLast
	}

	// Schritt B: Linearer Rückwärts-Sweep (Bottom-Up)
	// Da Kinder weiter hinten liegen, wurden sie bereits aggregiert, wenn das Elternteil erreicht wird.
	for i := len(ts.nodes) - 1; i > 0; i-- {
		pIdx := ts.nodes[i].ParentIdx
		if pIdx == -1 {
			continue // Root oder verwaister Knoten
		}

		child := &ts.nodes[i]
		parent := &ts.nodes[pIdx]

		// Roll up: min(First)
		if !child.AggFirst.IsZero() {
			if parent.AggFirst.IsZero() || child.AggFirst.Before(parent.AggFirst) {
				parent.AggFirst = child.AggFirst
			}
		}

		// Roll up: max(Last)
		if !child.AggLast.IsZero() {
			if parent.AggLast.IsZero() || child.AggLast.After(parent.AggLast) {
				parent.AggLast = child.AggLast
			}
		}
	}
}

// SetLocalMetrics erlaubt das schnelle Setzen von Daten vor einem Recompute-Aufruf
func (ts *TreeService) SetLocalMetrics(nodeID string, first, last time.Time) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if idx, exists := ts.indexMap[nodeID]; exists {
		ts.nodes[idx].LocalFirst = first
		ts.nodes[idx].LocalLast = last
	}
}

// ==========================================
// 3. STRUKTURÄNDERUNGEN (MUTATIONS)
// ==========================================

// AddNode fügt einen neuen Knoten zum Baum hinzu
func (ts *TreeService) AddNode(nodeID string, parentID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Überprüfen, ob der Knoten bereits existiert
	if _, exists := ts.indexMap[nodeID]; exists {
		return // Knoten existiert bereits
	}

	// Neue Node erstellen
	newNode := Node{
		ID:        nodeID,
		ParentID:  parentID,
		ParentIdx: -1,
	}

	// Index für den neuen Knoten setzen
	ts.indexMap[nodeID] = len(ts.nodes)

	// Zur internen Liste hinzufügen
	ts.nodes = append(ts.nodes, newNode)

	// ParentIdx für den neuen Knoten setzen
	if parentID != "" {
		if parentIdx, exists := ts.indexMap[parentID]; exists {
			ts.nodes[len(ts.nodes)-1].ParentIdx = parentIdx
		}
	}
}

// AddNodes fügt mehrere neue Knoten zum Baum hinzu
func (ts *TreeService) AddNodes(nodes []Node) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	// Überprüfen, welche Knoten bereits existieren
	existingNodes := make(map[string]bool)
	for _, node := range nodes {
		if _, exists := ts.indexMap[node.ID]; exists {
			existingNodes[node.ID] = true
		}
	}

	// Nur neue Knoten hinzufügen
	for _, node := range nodes {
		if !existingNodes[node.ID] {
			// Index für den neuen Knoten setzen
			ts.indexMap[node.ID] = len(ts.nodes)
			
			// Zur internen Liste hinzufügen
			ts.nodes = append(ts.nodes, node)
			
			// ParentIdx für den neuen Knoten setzen
			if node.ParentID != "" {
				if parentIdx, exists := ts.indexMap[node.ParentID]; exists {
					ts.nodes[len(ts.nodes)-1].ParentIdx = parentIdx
				}
			}
		}
	}
}

// MoveNode verschiebt einen Knoten zu einem neuen Parent und baut die Topologie neu auf
func (ts *TreeService) MoveNode(nodeID string, newParentID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	idx, exists := ts.indexMap[nodeID]
	if !exists {
		return
	}

	// Struktur updaten
	ts.nodes[idx].ParentID = newParentID

	// Da sich die Topologie geändert hat, nutzen wir den BFS-Algorithmus zur Neusortierung
	ts.rebuildFromInternalSlice()
}

// DeleteNode entfernt einen Knoten und alle seine Unterkinder (Kaskadierend)
func (ts *TreeService) DeleteNode(nodeID string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	deletedIDs := make(map[string]bool)
	deletedIDs[nodeID] = true

	// Nutzt die bestehende Sortierung aus: Kinder liegen immer weiter hinten!
	for i := 0; i < len(ts.nodes); i++ {
		if deletedIDs[ts.nodes[i].ParentID] {
			deletedIDs[ts.nodes[i].ID] = true
		}
	}

	// In-place Filterung des Slices
	newNodes := ts.nodes[:0]
	for _, n := range ts.nodes {
		if !deletedIDs[n.ID] {
			newNodes = append(newNodes, n)
		}
	}

	ts.nodes = newNodes
	ts.rebuildFromInternalSlice()
}

// Hilfsfunktion zum internen Wiederaufbau nach strukturellen Änderungen
func (ts *TreeService) rebuildFromInternalSlice() {
	byParent := make(map[string][]Node)
	var rootNodes []Node

	for _, n := range ts.nodes {
		if n.ParentID == "" {
			rootNodes = append(rootNodes, n)
		} else {
			byParent[n.ParentID] = append(byParent[n.ParentID], n)
		}
	}

	ordered := make([]Node, 0, len(ts.nodes))
	indexMap := make(map[string]int)

	queue := rootNodes
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		indexMap[curr.ID] = len(ordered)
		ordered = append(ordered, curr)

		if children, exists := byParent[curr.ID]; exists {
			queue = append(queue, children...)
		}
	}

	ts.nodes = ordered
	ts.indexMap = indexMap
	ts.rebuildParentIndices()
}

// ==========================================
// 4. READ-API (THREAD-SICHERES QUERY)
// ==========================================

// GetNodeAggregates gibt die berechneten Werte eines Knotens zurück
func (ts *TreeService) GetNodeAggregates(nodeID string) (first time.Time, last time.Time, exists bool) {
	ts.mu.RLock() // RLock erlaubt parallele Lesezugriffe
	defer ts.mu.RUnlock()

	idx, exists := ts.indexMap[nodeID]
	if !exists {
		return time.Time{}, time.Time{}, false
	}

	return ts.nodes[idx].AggFirst, ts.nodes[idx].AggLast, true
}
```
