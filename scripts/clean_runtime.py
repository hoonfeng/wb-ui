import os

runtime_dir = r'F:\syproject\wb-ui\jsc\runtime'
keep = {'jstype.go'}

for f in os.listdir(runtime_dir):
    if f.endswith('.go') and f not in keep:
        os.remove(os.path.join(runtime_dir, f))
        print(f"Deleted: {f}")

remaining = [f for f in os.listdir(runtime_dir) if f.endswith('.go')]
print(f"\nRemaining files: {remaining}")
