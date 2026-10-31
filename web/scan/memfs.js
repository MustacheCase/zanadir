// An in-memory file system with the callback API that Go's js/wasm port
// calls (Node's fs). Load it before wasm_exec.js, which otherwise installs a
// stub that fails every call.
"use strict";

(() => {
	const S_IFDIR = 0o040000;
	const S_IFREG = 0o100000;
	const constants = { O_RDONLY: 0, O_WRONLY: 1, O_RDWR: 2, O_CREAT: 64, O_EXCL: 128, O_TRUNC: 512, O_APPEND: 1024, O_DIRECTORY: 65536 };

	let nextIno = 1;
	const makeNode = (dir, mode) => ({
		dir, mode, ino: nextIno++, mtimeMs: Date.now(),
		children: dir ? new Map() : null, data: dir ? null : new Uint8Array(0), size: 0,
	});
	const root = makeNode(true, 0o755);

	const fail = (code) => {
		const err = new Error(code);
		err.code = code;
		return err;
	};
	const parts = (path) => {
		const out = [];
		for (const part of String(path).split("/")) {
			if (part === "" || part === ".") continue;
			if (part === "..") out.pop();
			else out.push(part);
		}
		return out;
	};
	const walk = (names) => {
		let node = root;
		for (const name of names) {
			if (!node.dir) throw fail("ENOTDIR");
			node = node.children.get(name);
			if (!node) throw fail("ENOENT");
		}
		return node;
	};
	const lookup = (path) => walk(parts(path));
	const parentOf = (path) => {
		const names = parts(path);
		const name = names.pop();
		if (name === undefined) throw fail("EEXIST");
		const dir = walk(names);
		if (!dir.dir) throw fail("ENOTDIR");
		return [dir, name];
	};
	const statOf = (node) => ({
		dev: 1, ino: node.ino, mode: (node.dir ? S_IFDIR : S_IFREG) | node.mode, nlink: 1,
		uid: 0, gid: 0, rdev: 0, size: node.dir ? 0 : node.size, blksize: 4096,
		blocks: node.dir ? 0 : Math.ceil(node.size / 512),
		atimeMs: node.mtimeMs, mtimeMs: node.mtimeMs, ctimeMs: node.mtimeMs,
		isDirectory: () => node.dir,
	});
	const grow = (node, size) => {
		if (size <= node.data.length) return;
		const data = new Uint8Array(Math.max(size, node.data.length * 2, 256));
		data.set(node.data.subarray(0, node.size));
		node.data = data;
	};
	const truncate = (node, size) => {
		if (node.dir) throw fail("EISDIR");
		grow(node, size);
		if (size > node.size) node.data.fill(0, node.size, size);
		node.size = size;
		node.mtimeMs = Date.now();
	};

	const files = new Map();
	let nextFd = 3;
	const fileOf = (fd) => {
		const file = files.get(fd);
		if (!file) throw fail("EBADF");
		return file;
	};

	// Turns a thrown error with a code into the callback's error; anything
	// else is a bug in this file and surfaces as one.
	const settle = (callback, body) => {
		let result;
		try {
			result = body();
		} catch (err) {
			if (!err.code) throw err;
			callback(err);
			return;
		}
		callback(null, result);
	};

	const decoder = new TextDecoder("utf-8");
	const pending = { 1: "", 2: "" };
	const writeSync = (fd, buf) => {
		pending[fd] += decoder.decode(buf);
		const nl = pending[fd].lastIndexOf("\n");
		if (nl !== -1) {
			(fd === 2 ? console.warn : console.log)(pending[fd].substring(0, nl));
			pending[fd] = pending[fd].substring(nl + 1);
		}
		return buf.length;
	};

	globalThis.fs = {
		constants,
		writeSync,
		open(path, flags, mode, callback) {
			settle(callback, () => {
				let node;
				try {
					node = lookup(path);
					if (flags & constants.O_CREAT && flags & constants.O_EXCL) throw fail("EEXIST");
				} catch (err) {
					if (err.code !== "ENOENT" || !(flags & constants.O_CREAT)) throw err;
					const [dir, name] = parentOf(path);
					node = makeNode(false, mode & 0o777);
					dir.children.set(name, node);
				}
				if (node.dir && flags & (constants.O_WRONLY | constants.O_RDWR)) throw fail("EISDIR");
				if (!node.dir && flags & constants.O_DIRECTORY) throw fail("ENOTDIR");
				if (!node.dir && flags & constants.O_TRUNC) truncate(node, 0);
				const fd = nextFd++;
				files.set(fd, { node, pos: 0, append: Boolean(flags & constants.O_APPEND) });
				return fd;
			});
		},
		close(fd, callback) {
			settle(callback, () => {
				fileOf(fd);
				files.delete(fd);
			});
		},
		read(fd, buffer, offset, length, position, callback) {
			settle(callback, () => {
				const file = fileOf(fd);
				if (file.node.dir) throw fail("EISDIR");
				const at = position ?? file.pos;
				const n = Math.max(0, Math.min(length, file.node.size - at));
				buffer.set(file.node.data.subarray(at, at + n), offset);
				if (position === null || position === undefined) file.pos += n;
				return n;
			});
		},
		write(fd, buffer, offset, length, position, callback) {
			if (fd === 1 || fd === 2) {
				callback(null, writeSync(fd, buffer.subarray(offset, offset + length)));
				return;
			}
			settle(callback, () => {
				const file = fileOf(fd);
				if (file.node.dir) throw fail("EISDIR");
				const at = position ?? (file.append ? file.node.size : file.pos);
				grow(file.node, at + length);
				file.node.data.set(buffer.subarray(offset, offset + length), at);
				file.node.size = Math.max(file.node.size, at + length);
				file.node.mtimeMs = Date.now();
				if (position === null || position === undefined) file.pos = at + length;
				return length;
			});
		},
		fstat(fd, callback) { settle(callback, () => statOf(fileOf(fd).node)); },
		stat(path, callback) { settle(callback, () => statOf(lookup(path))); },
		lstat(path, callback) { settle(callback, () => statOf(lookup(path))); },
		mkdir(path, perm, callback) {
			settle(callback, () => {
				const [dir, name] = parentOf(path);
				if (dir.children.has(name)) throw fail("EEXIST");
				dir.children.set(name, makeNode(true, perm & 0o777));
			});
		},
		readdir(path, callback) {
			settle(callback, () => {
				const node = lookup(path);
				if (!node.dir) throw fail("ENOTDIR");
				return [...node.children.keys()];
			});
		},
		rmdir(path, callback) {
			settle(callback, () => {
				const [dir, name] = parentOf(path);
				const node = dir.children.get(name);
				if (!node) throw fail("ENOENT");
				if (!node.dir) throw fail("ENOTDIR");
				if (node.children.size > 0) throw fail("ENOTEMPTY");
				dir.children.delete(name);
			});
		},
		unlink(path, callback) {
			settle(callback, () => {
				const [dir, name] = parentOf(path);
				const node = dir.children.get(name);
				if (!node) throw fail("ENOENT");
				if (node.dir) throw fail("EISDIR");
				dir.children.delete(name);
			});
		},
		rename(from, to, callback) {
			settle(callback, () => {
				const [fromDir, fromName] = parentOf(from);
				const node = fromDir.children.get(fromName);
				if (!node) throw fail("ENOENT");
				const [toDir, toName] = parentOf(to);
				const existing = toDir.children.get(toName);
				if (existing && existing.dir && existing.children.size > 0) throw fail("ENOTEMPTY");
				fromDir.children.delete(fromName);
				toDir.children.set(toName, node);
			});
		},
		truncate(path, length, callback) { settle(callback, () => truncate(lookup(path), length)); },
		ftruncate(fd, length, callback) { settle(callback, () => truncate(fileOf(fd).node, length)); },
		chmod(path, mode, callback) { settle(callback, () => { lookup(path).mode = mode & 0o777; }); },
		fchmod(fd, mode, callback) { settle(callback, () => { fileOf(fd).node.mode = mode & 0o777; }); },
		utimes(path, atime, mtime, callback) { settle(callback, () => { lookup(path).mtimeMs = mtime * 1000; }); },
		chown(path, uid, gid, callback) { settle(callback, () => { lookup(path); }); },
		fchown(fd, uid, gid, callback) { settle(callback, () => { fileOf(fd); }); },
		lchown(path, uid, gid, callback) { settle(callback, () => { lookup(path); }); },
		fsync(fd, callback) { settle(callback, () => { fileOf(fd); }); },
		readlink(path, callback) { settle(callback, () => { lookup(path); throw fail("EINVAL"); }); },
		symlink(target, path, callback) { callback(fail("ENOSYS")); },
		link(from, to, callback) { callback(fail("ENOSYS")); },
	};

	globalThis.process = {
		getuid: () => -1, getgid: () => -1, geteuid: () => -1, getegid: () => -1,
		getgroups: () => [], pid: 1, ppid: 0,
		umask: () => 0o022,
		cwd: () => "/",
		chdir: () => { throw fail("ENOSYS"); },
	};

	root.children.set("tmp", makeNode(true, 0o777));
})();
