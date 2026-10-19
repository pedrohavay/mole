package main

/*
#include <stdint.h>
*/
import "C"

import (
	"errors"
	"time"

	"mole/core/internal/bookfile"
	"mole/core/internal/importer"
	"mole/core/internal/library"
)

// Collections and watched folders.

// libraryStore returns the store of a library handle, recording an error
// for an unknown handle.
func libraryStore(lib C.int64_t) (*library.Store, bool) {
	store, ok := stores.get(int64(lib))
	if !ok {
		fail(errBadHandle)
	}
	return store, ok
}

type collectionJSON struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Folder string `json:"folder"` // the folder it was made from, or ""
}

type folderJSON struct {
	ID          int64  `json:"id"`
	Path        string `json:"path"`
	Collections bool   `json:"collections"` // its subfolders become collections
	AddedAt     int64  `json:"addedAt"`     // Unix seconds
	SyncedAt    int64  `json:"syncedAt"`    // Unix seconds, 0 before the first sync
}

type failureJSON struct {
	Path  string `json:"path"`
	Error string `json:"error"`
	DRM   bool   `json:"drm"` // the book is protected by DRM
}

type folderResultJSON struct {
	Folder  int64         `json:"folder"` // the watched folder, or 0
	Found   int           `json:"found"`
	Added   []int64       `json:"added"`
	Updated []int64       `json:"updated"`
	Missing []int64       `json:"missing"`
	Failed  []failureJSON `json:"failed"`
}

// folderOptionsJSON are the options of mole_library_import_folder.
type folderOptionsJSON struct {
	Collections bool `json:"collections"`
	Watch       bool `json:"watch"`
}

func toFolderResultJSON(r importer.FolderResult) folderResultJSON {
	out := folderResultJSON{Folder: r.Folder, Found: r.Found, Added: ids(r.Added), Updated: ids(r.Updated),
		Missing: ids(r.Missing), Failed: make([]failureJSON, 0, len(r.Failed))}
	for _, f := range r.Failed {
		out.Failed = append(out.Failed, failureJSON{Path: f.Path, Error: f.Err.Error(), DRM: errors.Is(f.Err, bookfile.ErrDRM)})
	}
	return out
}

// ids returns list, or an empty list for nil, so JSON gets [] rather than
// null.
func ids(list []int64) []int64 {
	if list == nil {
		return []int64{}
	}
	return list
}

// mole_library_collections returns the collections by name, as a JSON array
// of {"id","name","folder"}; folder is the folder a collection was made
// from, or "". Books list the collections that hold them.
//
//export mole_library_collections
func mole_library_collections(lib C.int64_t) *C.char {
	store, ok := libraryStore(lib)
	if !ok {
		return nil
	}
	list, err := store.Collections()
	if err != nil {
		fail(err)
		return nil
	}
	out := make([]collectionJSON, 0, len(list))
	for _, c := range list {
		out = append(out, collectionJSON{ID: c.ID, Name: c.Name, Folder: c.Folder})
	}
	return jsonString(out)
}

// mole_library_create_collection makes an empty collection and returns its
// ID, or 0. It fails when another collection has the name, compared without
// regard to ASCII case.
//
//export mole_library_create_collection
func mole_library_create_collection(lib C.int64_t, name *C.char) C.int64_t {
	store, ok := libraryStore(lib)
	if !ok {
		return 0
	}
	c, err := store.CreateCollection(C.GoString(name), time.Now())
	if err != nil {
		fail(err)
		return 0
	}
	return C.int64_t(c.ID)
}

// mole_library_rename_collection renames a collection and returns 1, or 0
// on error.
//
//export mole_library_rename_collection
func mole_library_rename_collection(lib, id C.int64_t, name *C.char) C.int {
	store, ok := libraryStore(lib)
	if !ok {
		return 0
	}
	if err := store.RenameCollection(int64(id), C.GoString(name)); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// mole_library_delete_collection deletes a collection, not its books, and
// returns 1, or 0 on error.
//
//export mole_library_delete_collection
func mole_library_delete_collection(lib, id C.int64_t) C.int {
	store, ok := libraryStore(lib)
	if !ok {
		return 0
	}
	if err := store.DeleteCollection(int64(id)); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// mole_library_add_to_collection puts a book in a collection and returns 1,
// or 0 on error. A book already there stays as it is.
//
//export mole_library_add_to_collection
func mole_library_add_to_collection(lib, collection, book C.int64_t) C.int {
	store, ok := libraryStore(lib)
	if !ok {
		return 0
	}
	if err := store.AddToCollection(int64(collection), int64(book), time.Now()); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// mole_library_remove_from_collection takes a book out of a collection and
// returns 1, or 0 on error.
//
//export mole_library_remove_from_collection
func mole_library_remove_from_collection(lib, collection, book C.int64_t) C.int {
	store, ok := libraryStore(lib)
	if !ok {
		return 0
	}
	if err := store.RemoveFromCollection(int64(collection), int64(book)); err != nil {
		fail(err)
		return 0
	}
	return 1
}

// mole_library_folders returns the watched folders by path, as a JSON array
// of {"id","path","collections","addedAt","syncedAt"}, times in Unix
// seconds.
//
//export mole_library_folders
func mole_library_folders(lib C.int64_t) *C.char {
	store, ok := libraryStore(lib)
	if !ok {
		return nil
	}
	list, err := store.Folders()
	if err != nil {
		fail(err)
		return nil
	}
	out := make([]folderJSON, 0, len(list))
	for _, f := range list {
		item := folderJSON{ID: f.ID, Path: f.Path, Collections: f.Collections, AddedAt: f.AddedAt.Unix()}
		if !f.SyncedAt.IsZero() {
			item.SyncedAt = f.SyncedAt.Unix()
		}
		out = append(out, item)
	}
	return jsonString(out)
}

// mole_library_import_folder imports every book (EPUB, MOBI, PRC, FB2 or
// plain text) in a folder and its subfolders. options is JSON {"collections":bool,"watch":bool}: collections
// puts each book in a collection named after its folder, and watch keeps
// the folder in sync from then on (see mole_library_sync_folder). It
// returns JSON {"folder","found","added","updated","missing","failed"}:
// the watched folder's ID or 0, how many files were found, the IDs of the
// books added, updated and marked missing, and the files that failed as
// {"path","error","drm"}, drm being true for a book protected by DRM. It may run on a background thread.
//
//export mole_library_import_folder
func mole_library_import_folder(lib C.int64_t, path, options *C.char) *C.char {
	store, ok := libraryStore(lib)
	if !ok {
		return nil
	}
	var opts folderOptionsJSON
	if !parseJSON(options, "folder options", &opts) {
		return nil
	}
	r, err := importer.ImportFolder(store, C.GoString(path),
		importer.FolderOptions{Collections: opts.Collections, Watch: opts.Watch}, time.Now())
	if err != nil {
		fail(err)
		return nil
	}
	return jsonString(toFolderResultJSON(r))
}

// mole_library_sync_folder brings the library up to date with a watched
// folder: new and changed files are imported, and books whose file left
// the folder are marked missing, never removed. It returns the same JSON as
// mole_library_import_folder, and may run on a background thread.
//
//export mole_library_sync_folder
func mole_library_sync_folder(lib, folder C.int64_t) *C.char {
	store, ok := libraryStore(lib)
	if !ok {
		return nil
	}
	r, err := importer.Sync(store, int64(folder), time.Now())
	if err != nil {
		fail(err)
		return nil
	}
	return jsonString(toFolderResultJSON(r))
}

// mole_library_remove_folder stops watching a folder and returns 1, or 0 on
// error. Its books stay in the library.
//
//export mole_library_remove_folder
func mole_library_remove_folder(lib, folder C.int64_t) C.int {
	store, ok := libraryStore(lib)
	if !ok {
		return 0
	}
	if err := store.RemoveFolder(int64(folder)); err != nil {
		fail(err)
		return 0
	}
	return 1
}
