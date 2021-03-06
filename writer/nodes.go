package writer

import (
	"imposm3/cache"
	"imposm3/database"
	"imposm3/element"
	"imposm3/geom"
	"imposm3/geom/geos"
	"imposm3/mapping"
	"imposm3/proj"
	"imposm3/stats"
	"log"
	"sync"
)

type NodeWriter struct {
	OsmElemWriter
	nodes      chan *element.Node
	tagMatcher *mapping.TagMatcher
}

func NewNodeWriter(osmCache *cache.OSMCache, nodes chan *element.Node,
	insertBuffer database.RowInserter, tagMatcher *mapping.TagMatcher, progress *stats.Statistics,
	srid int) *OsmElemWriter {
	nw := NodeWriter{
		OsmElemWriter: OsmElemWriter{
			osmCache:     osmCache,
			progress:     progress,
			wg:           &sync.WaitGroup{},
			insertBuffer: insertBuffer,
			srid:         srid,
		},
		nodes:      nodes,
		tagMatcher: tagMatcher,
	}
	nw.OsmElemWriter.writer = &nw
	return &nw.OsmElemWriter
}

func (nw *NodeWriter) loop() {
	geos := geos.NewGeos()
	geos.SetHandleSrid(nw.srid)
	defer geos.Finish()

	for n := range nw.nodes {
		nw.progress.AddNodes(1)
		if matches := nw.tagMatcher.Match(&n.Tags); len(matches) > 0 {
			proj.NodeToMerc(n)
			if nw.expireTiles != nil {
				nw.expireTiles.ExpireFromNodes([]element.Node{*n})
			}
			point, err := geom.Point(geos, *n)
			if err != nil {
				if err, ok := err.(ErrorLevel); ok {
					if err.Level() <= 0 {
						continue
					}
				}
				log.Println(err)
				continue
			}

			n.Geom, err = geom.AsGeomElement(geos, point)
			if err != nil {
				log.Println(err)
				continue
			}

			if nw.limiter != nil {
				parts, err := nw.limiter.Clip(n.Geom.Geom)
				if err != nil {
					log.Println(err)
					continue
				}
				if len(parts) >= 1 {
					nw.insertMatches(&n.OSMElem, matches)
				}
			} else {
				nw.insertMatches(&n.OSMElem, matches)
			}

		}
	}
	nw.wg.Done()
}
