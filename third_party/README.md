# Third Party Package Patching

This directory includes local patches to third party dependencies we use in Prysm. Sometimes,
we need to make a small change to some dependency for ease of use in Prysm without wanting
to maintain our own fork of the dependency ourselves.

**Given maintaining a patch can be difficult and tedious,
patches are NOT the recommended way of modifying dependencies in Prysm
unless really needed**

## Table of Contents

- [Prerequisites](#prerequisites)
- [Creating a Patch](#creating-a-patch)

## Prerequisites

- A modern UNIX operating system (macOS included)
- Go installed

## Creating a Patch

To create a patch, we need an original version of a dependency which we will refer to as `a`
and the patched version referred to as `b`.

```
cd /tmp
git clone https://github.com/someteam/somerepo a
git clone https://github.com/someteam/somerepo b && cd b
```
Then, make all your changes in `b` and finally create the diff of all your changes as follows:
```
cd ..
diff -ur --exclude=".git" a b > $GOPATH/src/github.com/prysmaticlabs/prysm/third_party/YOURPATCH.patch
```
