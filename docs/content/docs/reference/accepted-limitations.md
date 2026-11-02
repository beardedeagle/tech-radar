---
title: "Accepted limitations"
weight: 80
---

# Accepted limitations

## Concurrent edits by another program

**Won't fix.** These two cases apply when `--force` replaces an existing generated report or context file.

tech-radar locks the output before reading saved state and checks the original file's identity and content before replacement. That protects against another tech-radar run. Editors and other programs can ignore the lock.

### A save lands between the final check and replacement

Another program must write to or replace the same output after the relevant content has been read for the final check, but before rename replaces the file. The generated export can overwrite that save. The interval is normally brief, though process suspension can extend it; there is no fixed upper bound.

### A program keeps the old file open for writing

Another program can open the output for writing, keep that descriptor across replacement and write through it afterward. Where the filesystem permits this, the descriptor still refers to the old file object. The later edit may no longer be accessible through the output filename and can disappear when the last descriptor closes.

Having the document open for reading, or having unsaved text in an editor buffer, isn't enough. The program must keep a writable descriptor open across replacement.

### Impact and likelihood

The affected file is the chosen output. A lost edit could include report prose, saved decisions or generated context. Scanning reads application source without changing it. The report doesn't install technologies, change application behavior or deploy anything.

We expect these cases to be rare in the usual generate, review and PR workflow. We haven't measured their frequency or recorded a real-world occurrence; controlled tests show that they are possible.

Separate clones, different output filenames and regenerating one report at a time don't create this collision. A new-file write refuses to overwrite an existing file. Detected edits stop replacement, and other tech-radar writers are excluded by the lock.

### Why this is left unfixed

The expected likelihood is low, the direct impact is limited to generated files, and the normal workflow reviews those files before accepting policy changes. That doesn't justify adding recovery storage, cleanup rules and separate replacement protocols for each platform.

The ordinary portable APIs we examined can't check expected content and replace the filename in one operation. Keeping the replaced file could make edits recoverable, but it creates more files and changes what happens on a conflict. A timeout can't prove an old writable descriptor will never write again, so cleanup would need its own rules. The added complexity outweighs the risk.

Keep decisions in policy files, regenerate exports and avoid writing to an export from another program while replacing it. Use a distinct output filename when concurrent editing is necessary.

### What this decision covers

These two cases are closed as won't fix. Collisions between tech-radar writers, bypassed validation, overwritten inputs, writes outside the intended output paths and regressions with materially greater impact remain bugs.

This doesn't close native-platform or network-filesystem verification gaps; see [testing](../contributors/testing.md).
