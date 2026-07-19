import os

rt = r'F:\syproject\wb-ui\jsc\runtime'
keep = {'jstype.go'}
deleted = []
for f in os.listdir(rt):
    if f.endswith('.go') and f not in keep:
        os.remove(os.path.join(rt, f))
        deleted.append(f)
print(f"Deleted {len(deleted)} files")
for f in sorted(deleted):
    print(f"  {f}")
print(f"\nRemaining: {[f for f in os.listdir(rt) if f.endswith('.go')]}")
