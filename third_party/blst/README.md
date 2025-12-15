# blst BUILD file

Due to the project structure of [blst](https://github.com/supranational/blst) having go bindings 
and cdeps in different directories, [gazelle](https://github.com/bazel-contrib/bazel-gazelle) 
is unable to appropriately generate the BUILD.bazel files for this repository. We have hand written
the BUILD.bazel file here by the name `blst.BUILD`. PR [#6539](https://github.com/OffchainLabs/prysm/pull/6539)
added build support for blst, but relied on an [http_archive](https://bazel.build/rules/lib/repo/http#http_archive)
repository rule to provide blst as a dependency. This pattern worked, but gazelle would not keep the 
dependency in sync with go.mod. There was a risk that go and bazel builds would include different versions
of blst. 

Now, we can switch to a [go_repository](https://github.com/bazel-contrib/bazel-gazelle/blob/master/reference.md#go_repository)
model which gazelle understand how to sync with go.mod. However, we still have to tell gazelle how generate a BUILD.bazel file.
Our solution is to tell gazelle not to generate any build file, then we provide blst.BUILD as a patch. 

Generating the patch is relatively straight forward:

```
mkdir /tmp/a
mkdir /tmp/b
cp ./third_party/blst/blst.BUILD /tmp/b/BUILD.bazel
(cd /tmp && diff -urN a b) > ./third_party/com_github_supranational_blst.patch
```

If future edits are needed, edit the ./third_party/blst/blst.BUILD and regenerate the patch.
