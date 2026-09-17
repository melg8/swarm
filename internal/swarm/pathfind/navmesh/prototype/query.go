// SPDX-FileCopyrightText: 2026 Melg Eight <public.melg8@gmail.com>
//
// SPDX-License-Identifier: MIT

package prototype

import (
	"container/heap"
	"math"
)

// Query answers navigation queries over one parsed tile. The area
// costs mirror the dtQueryFilter semantics: entering a polygon of an
// area multiplies the distance cost of the step by the area cost.
type Query struct {
	tile     *Tile
	areaCost [64]float32
}

// NewQuery creates a query engine over a tile with every area cost
// set to one.
func NewQuery(tile *Tile) *Query {
	query := &Query{tile: tile, areaCost: [64]float32{}}
	for i := range query.areaCost {
		query.areaCost[i] = 1
	}

	return query
}

// SetAreaCost overrides the cost multiplier of one area id.
func (q *Query) SetAreaCost(area int, cost float32) {
	if area >= 0 && area < len(q.areaCost) {
		q.areaCost[area] = cost
	}
}

// Point is a world position in the Recast axis order of the tile
// export (x, height, world y).
type Point [3]float64

// FindNearestPoly returns the polygon of the tile whose surface is
// the closest to the query point together with the closest surface
// point, exactly the dtNavMeshQuery::findNearestPoly semantics that
// disambiguate stacked layers by the 3D distance: a point under the
// bridge binds to the water polygon, the same x/y at deck height to
// the bridge deck polygon.
func (q *Query) FindNearestPoly(point Point) (int, Point, bool) {
	const halfExtentXZ = 32
	const halfExtentY = 600
	candidates := q.queryPolys(point, halfExtentXZ, halfExtentY)
	bestPoly := -1
	var best Point
	bestDist := math.MaxFloat64
	for _, poly := range candidates {
		closest := q.closestPointOnPoly(poly, point)
		dist := distSq3(point, closest)
		if dist < bestDist {
			bestDist = dist
			bestPoly = poly
			best = closest
		}
	}

	return bestPoly, best, bestPoly >= 0
}

// queryPolys walks the bounding volume tree and returns the polygon
// indices whose quantized bounds overlap the query box around the
// point (the dtNavMeshQuery::queryPolygonsInTile traversal).
func (q *Query) queryPolys(point Point, halfXZ, halfY int) []int {
	tree := q.tile.BVTree
	if len(tree) == 0 {
		// Without a tree answer with every polygon: correct, slow.
		all := make([]int, len(q.tile.Polys))
		for i := range all {
			all[i] = i
		}

		return all
	}
	header := &q.tile.Header
	qfac := float64(header.BVQuantFactor)
	var qmin [3]float64
	var qmax [3]float64
	qmin[0] = point[0] - float64(halfXZ)
	qmax[0] = point[0] + float64(halfXZ)
	qmin[1] = point[1] - float64(halfY)
	qmax[1] = point[1] + float64(halfY)
	qmin[2] = point[2] - float64(halfXZ)
	qmax[2] = point[2] + float64(halfXZ)
	var bmin [3]uint16
	var bmax [3]uint16
	for a := range 3 {
		lo := qmin[a] - float64(header.BMin[a])
		hi := qmax[a] - float64(header.BMin[a])
		lo = math.Max(lo, 0)
		hi = math.Min(hi, float64(header.BMax[a])-float64(header.BMin[a]))
		// The even/odd lowest bit keeps overlapping quantized bounds
		// strictly separated, the Detour dtOverlapQuantBounds trick.
		bmin[a] = quantize(lo*qfac) & 0xFFFE
		bmax[a] = quantize(hi*qfac+1) | 1
	}
	found := make([]int, 0, 32)
	i := 0
	for i < len(tree) {
		node := &tree[i]
		overlap := quantOverlap(bmin, bmax, node.BMin, node.BMax)
		leaf := node.I >= 0
		if leaf && overlap {
			found = append(found, int(node.I))
		}
		if overlap || leaf {
			i++
		} else {
			i += int(-node.I)
		}
	}

	return found
}

// quantize clamps and truncates a quantized bound the same way the
// C++ cast to unsigned short does.
func quantize(value float64) uint16 {
	if value < 0 {
		return 0
	}
	if value > 65535 {
		return 65535
	}

	return uint16(value)
}

// quantOverlap reports whether two quantized bounds overlap.
func quantOverlap(aMin, aMax, bMin, bMax [3]uint16) bool {
	for a := range 3 {
		if aMin[a] > bMax[a] || aMax[a] < bMin[a] {
			return false
		}
	}

	return true
}

// closestPointOnPoly projects a point onto the polygon surface: when
// the point lies inside the polygon footprint the answer is the
// footprint position with the interpolated surface height, otherwise
// the closest point of the boundary.
func (q *Query) closestPointOnPoly(poly int, point Point) Point {
	p := &q.tile.Polys[poly]
	verts := q.tile.Verts
	vx := func(i int) float64 {
		return float64(verts[p.Verts[i]*3])
	}
	vy := func(i int) float64 {
		return float64(verts[p.Verts[i]*3+1])
	}
	vz := func(i int) float64 {
		return float64(verts[p.Verts[i]*3+2])
	}
	if pointInPoly(p, vx, vz, point[0], point[2]) {
		return Point{point[0], q.polyHeight(p, vx, vy, vz, point[0],
			point[2]), point[2]}
	}
	// Outside: the closest boundary edge point with the edge height.
	best := Point{}
	bestDist := math.MaxFloat64
	for e := range p.VertCount {
		next := (e + 1) % p.VertCount
		ax, az, ay := vx(e), vz(e), vy(e)
		bx, bz := vx(next), vz(next)
		t := 0.0
		dx := bx - ax
		dz := bz - az
		denom := dx*dx + dz*dz
		if denom > 1e-12 {
			t = ((point[0]-ax)*dx + (point[2]-az)*dz) / denom
			t = math.Max(0, math.Min(1, t))
		}
		cx := ax + dx*t
		cz := az + dz*t
		cy := ay + (vy(next)-ay)*t
		dist := distSq3(point, Point{cx, cy, cz})
		if dist < bestDist {
			bestDist = dist
			best = Point{cx, cy, cz}
		}
	}

	return best
}

// pointInPoly is the crossing number containment test over the
// polygon footprint in the x/z plane.
func pointInPoly(p *Poly, vx, vz func(int) float64, px, pz float64) bool {
	inside := false
	j := p.VertCount - 1
	for i := range p.VertCount {
		if (vz(i) > pz) != (vz(j) > pz) &&
			px < (vx(j)-vx(i))*(pz-vz(i))/(vz(j)-vz(i))+vx(i) {
			inside = !inside
		}
		j = i
	}

	return inside
}

// polyHeight interpolates the surface height at the footprint
// position over the polygon triangle fan.
func (q *Query) polyHeight(p *Poly, vx, vy, vz func(int) float64,
	px, pz float64,
) float64 {
	for i := 1; i+1 < p.VertCount; i++ {
		ax, az := vx(0), vz(0)
		bx, bz := vx(i), vz(i)
		cx, cz := vx(i+1), vz(i+1)
		d := (bz-cz)*(ax-cx) + (cx-bx)*(az-cz)
		if math.Abs(d) < 1e-12 {
			continue
		}
		lambda1 := ((bz-cz)*(px-cx) + (cx-bx)*(pz-cz)) / d
		lambda2 := ((cz-az)*(px-cx) + (ax-cx)*(pz-cz)) / d
		lambda3 := 1 - lambda1 - lambda2
		if lambda1 >= -1e-9 && lambda2 >= -1e-9 && lambda3 >= -1e-9 {
			return lambda1*vy(0) + lambda2*vy(i) + lambda3*vy(i+1)
		}
	}
	// Degenerate fan: the average vertex height.
	sum := 0.0
	for i := range p.VertCount {
		sum += vy(i)
	}

	return sum / float64(p.VertCount)
}

// PathNode is one A* node: the polygon, the entry position Detour
// uses (the shared edge midpoint), the parent polygon and the
// bookkeeping.
type PathNode struct {
	Poly    int
	Parent  int
	Pos     Point
	G, F    float64
	Index   int
	Settled bool
}

// FindPath searches the polygon corridor from startPoly to endPoly
// with the A* of dtNavMeshQuery::findPath: the node position is the
// shared edge midpoint, the step cost is the averaged area cost of
// the two polygons times the traveled distance and the heuristic is
// the straight 3D distance to the end point. The answer is nil when
// the target is unreachable.
func (q *Query) FindPath(startPoly, endPoly int, start, end Point) []int {
	if startPoly == endPoly {
		return []int{startPoly}
	}
	nodes := make([]PathNode, len(q.tile.Polys))
	for i := range nodes {
		nodes[i].Poly = i
		nodes[i].Parent = -1
		nodes[i].G = math.MaxFloat64
		nodes[i].F = math.MaxFloat64
		nodes[i].Index = -1
		nodes[i].Settled = false
	}
	nodes[startPoly].G = 0
	nodes[startPoly].F = dist3(start, end)
	nodes[startPoly].Pos = start
	open := make(nodeHeap, 0, 64)
	heap.Push(&open, &nodes[startPoly])
	for open.Len() > 0 {
		current := heap.Pop(&open).(*PathNode)
		if current.Settled {
			continue
		}
		current.Settled = true
		if current.Poly == endPoly {
			return q.reconstruct(nodes, endPoly)
		}
		for edge, neighborIdx := range q.tile.Adjacency[current.Poly] {
			if neighborIdx < 0 {
				continue
			}
			neighbor := int(neighborIdx)
			if nodes[neighbor].Settled {
				continue
			}
			mid := q.edgeMidpoint(current.Poly, edge)
			stepCost := q.stepCost(current.Poly, neighbor, current.Pos,
				mid)
			step := current.G + stepCost
			if step >= nodes[neighbor].G {
				continue
			}
			nodes[neighbor].G = step
			nodes[neighbor].F = step + dist3(mid, end)
			nodes[neighbor].Parent = current.Poly
			nodes[neighbor].Pos = mid
			if nodes[neighbor].Index >= 0 {
				heap.Fix(&open, nodes[neighbor].Index)
			} else {
				heap.Push(&open, &nodes[neighbor])
			}
		}
	}

	return nil
}

// edgeMidpoint returns the world midpoint of one polygon edge.
func (q *Query) edgeMidpoint(poly, edge int) Point {
	p := &q.tile.Polys[poly]
	next := (edge + 1) % p.VertCount
	verts := q.tile.Verts
	a := p.Verts[edge] * 3
	b := p.Verts[next] * 3

	return Point{
		(float64(verts[a]) + float64(verts[b])) * 0.5,
		(float64(verts[a+1]) + float64(verts[b+1])) * 0.5,
		(float64(verts[a+2]) + float64(verts[b+2])) * 0.5,
	}
}

// stepCost prices one polygon-to-polygon step: the averaged area
// costs of the two polygons times the distance between the entry
// position and the shared edge midpoint.
func (q *Query) stepCost(from, to int, fromPos, mid Point) float64 {
	cost := (float64(q.areaCost[q.tile.Polys[from].Area]) +
		float64(q.areaCost[q.tile.Polys[to].Area])) * 0.5

	return cost * dist3(fromPos, mid)
}

// reconstruct walks the parent chain back from the target and
// reverses it into the start to end corridor.
func (q *Query) reconstruct(nodes []PathNode, endPoly int) []int {
	depth := 0
	for node := endPoly; node >= 0; node = nodes[node].Parent {
		depth++
	}
	path := make([]int, depth)
	for node := endPoly; node >= 0; node = nodes[node].Parent {
		depth--
		path[depth] = node
	}

	return path
}

// Corners turns a polygon corridor into walk waypoints: the start,
// the midpoint of every shared edge crossing and the end. This is
// the simplified string pulling of the prototype - the production
// port would carry the full funnel algorithm of findStraightPath.
func (q *Query) Corners(path []int, start, end Point) []Point {
	if len(path) == 0 {
		return nil
	}
	corners := make([]Point, 0, len(path)+1)
	corners = append(corners, start)
	for i := 0; i+1 < len(path); i++ {
		edge := q.sharedEdge(path[i], path[i+1])
		if edge >= 0 {
			corners = append(corners, q.edgeMidpoint(path[i], edge))
		}
	}
	corners = append(corners, end)

	return corners
}

// sharedEdge finds the edge of poly that leads to the neighbor.
func (q *Query) sharedEdge(poly, neighbor int) int {
	for edge, candidate := range q.tile.Adjacency[poly] {
		if int(candidate) == neighbor {
			return edge
		}
	}

	return -1
}

// nodeHeap is the A* open set.
type nodeHeap []*PathNode

func (h nodeHeap) Len() int { return len(h) }

func (h nodeHeap) Less(i, j int) bool { return h[i].F < h[j].F }

func (h nodeHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].Index = i
	h[j].Index = j
}

func (h *nodeHeap) Push(x any) {
	n := x.(*PathNode)
	n.Index = len(*h)
	*h = append(*h, n)
}

func (h *nodeHeap) Pop() any {
	old := *h
	item := old[len(old)-1]
	item.Index = -1
	*h = old[:len(old)-1]

	return item
}

// distSq3 is the squared 3D distance.
func distSq3(a, b Point) float64 {
	dx := a[0] - b[0]
	dy := a[1] - b[1]
	dz := a[2] - b[2]

	return dx*dx + dy*dy + dz*dz
}

// dist3 is the 3D distance.
func dist3(a, b Point) float64 {
	return math.Sqrt(distSq3(a, b))
}
