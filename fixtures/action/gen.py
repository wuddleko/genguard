from pathlib import Path

root = Path(__file__).resolve().parent
src = (root / "name.txt").read_text(encoding="utf-8").strip()
(root / "hello.txt").write_bytes(f"hello {src}\n".encode("utf-8"))
