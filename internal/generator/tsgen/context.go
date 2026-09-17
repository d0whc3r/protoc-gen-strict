package tsgen

import (
	"strings"

	"google.golang.org/protobuf/compiler/protogen"

	"github.com/d0whc3r/protoc-gen-strict/internal/parser"
)

// Context is one resolution pass over every message the plugin was asked to
// generate, so the emitters print without looking anything up again.
//
// It spans files on purpose: a narrowing reaches through message-typed fields,
// so a message in one file can be why a message in another needs a strict type.
// Hence `strategy: all` in buf.gen.yaml, since under directory sharding the run
// that did not see the target would silently drop the narrowing.
type Context struct {
	messages map[string]parser.MessageMetadata // by fully qualified proto name
	fileOf   map[string]string                 // fq name -> proto file path
	pkgOf    map[string]string                 // proto file path -> proto package
	byFile   map[string][]parser.MessageMetadata

	// What each message's CEL rules did: the narrowing they imply, and a record
	// per rule for the generated doc.
	celTrees map[string]*celNode
	celNotes map[string][]celNote

	// Messages that get a <Name>Strict type: those with a rule of their own, and
	// those reaching one through a message-typed field.
	needsStrict map[string]bool
	// Messages that also get a <Name>StrictSchema: an RPC input or output that
	// narrows. Any other message would get a const nothing dispatches on.
	strictSchemas map[string]bool
}

// New resolves every message the run parsed, once. files is the whole request,
// because a narrowing reaches across files; parsed holds only the generated
// ones, keyed by proto path, which is the set this run can narrow.
func New(files []*protogen.File, parsed map[string][]parser.MessageMetadata) *Context {
	c := &Context{
		messages:      map[string]parser.MessageMetadata{},
		fileOf:        map[string]string{},
		pkgOf:         map[string]string{},
		byFile:        map[string][]parser.MessageMetadata{},
		celTrees:      map[string]*celNode{},
		celNotes:      map[string][]celNote{},
		needsStrict:   map[string]bool{},
		strictSchemas: map[string]bool{},
	}

	// Where a type is declared matters for every file in the request, not only
	// the generated ones: an RPC can take a message from a file nobody asked to
	// generate, and the overlay still has to name its module.
	for _, file := range files {
		c.pkgOf[file.Desc.Path()] = string(file.Desc.Package())
		indexMessages(c.fileOf, file.Desc.Path(), file.Messages)
	}

	for _, file := range files {
		messages, ok := parsed[file.Desc.Path()]
		if !ok {
			continue
		}
		c.byFile[file.Desc.Path()] = messages
		for _, msg := range messages {
			c.messages[msg.Name] = msg
		}
	}

	// A CEL path reaches through message-typed fields, so fold the trees only
	// once every parsed message is known.
	for name, msg := range c.messages {
		c.celTrees[name], c.celNotes[name] = c.buildCELTree(msg)
	}

	// A message needs a strict type if it narrows itself or reaches one that
	// does, so close the set under the second rule.
	//
	// ponytail: re-scans every message per round instead of walking reverse
	// edges. Small schemas, once per generation; add a reverse index if it ever
	// shows up in a profile.
	for changed := true; changed; {
		changed = false
		for name, msg := range c.messages {
			if c.needsStrict[name] {
				continue
			}
			if !c.narrowsItself(msg) && !c.reachesStrict(msg) {
				continue
			}
			c.needsStrict[name] = true
			changed = true
		}
	}

	for _, file := range files {
		if !file.Generate {
			continue
		}
		for _, service := range file.Services {
			for _, method := range service.Methods {
				for _, name := range []string{string(method.Input.Desc.FullName()), string(method.Output.Desc.FullName())} {
					if c.needsStrict[name] {
						c.strictSchemas[name] = true
					}
				}
			}
		}
	}
	return c
}

// indexMessages records the file each message is declared in, nested included.
func indexMessages(fileOf map[string]string, path string, msgs []*protogen.Message) {
	for _, msg := range msgs {
		fileOf[string(msg.Desc.FullName())] = path
		indexMessages(fileOf, path, msg.Messages)
	}
}

// narrowsItself reports whether a rule changes the message's own type, ignoring
// what its fields' targets do.
func (c *Context) narrowsItself(msg parser.MessageMetadata) bool {
	if !c.celTrees[msg.Name].empty() {
		return true
	}
	for _, oneof := range msg.Oneofs {
		if oneof.Name != "" && oneof.Required {
			return true
		}
	}
	for _, field := range msg.Fields {
		if field.OneofName != "" {
			continue
		}
		if !c.fieldNarrowing(field).isZero() {
			return true
		}
	}
	return false
}

// reachesStrict reports whether a message-typed field of msg points at one that
// already needs a strict type.
func (c *Context) reachesStrict(msg parser.MessageMetadata) bool {
	for _, field := range msg.Fields {
		if target, ok := messageTarget(field); ok && c.needsStrict[target] {
			return true
		}
	}
	return false
}

// messageTarget returns the message a field carries, singular or repeated.
//
// ponytail: a map with a message value is not followed. protoc-gen-es types
// maps as an index signature, so narrowing the value would need a mapped type;
// add one if a schema puts rules there.
func messageTarget(field parser.FieldMetadata) (string, bool) {
	if field.IsMap {
		return "", false
	}
	name, ok := strings.CutPrefix(field.ProtoType, "message:")
	return name, ok
}

// tsName is tsIdent for a message, resolving its package through the index.
func (c *Context) tsName(fullName string) string {
	return tsIdent(c.pkgOf[c.fileOf[fullName]], fullName)
}

func (c *Context) strictName(fullName string) string {
	return c.tsName(fullName) + "Strict"
}
