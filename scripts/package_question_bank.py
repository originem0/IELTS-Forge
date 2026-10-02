"""Build one local import ZIP from packs/ and content-addressed media/."""
import argparse
import collections
import hashlib
import json
import pathlib
import re
import zipfile


def package(source, output):
    packs = sorted((source / 'packs').glob('*.json'))
    if not packs:
        raise ValueError('No question packs found')
    counts, parts = collections.Counter(), collections.Counter()
    refs, identities = set(), set()
    points = exams = 0
    for path in packs:
        pack = json.loads(path.read_text(encoding='utf-8'))
        exams += len(pack.get('exams', []))
        for unit in pack['units']:
            identity = (pack['source'].get('url') or pack['source']['name'], unit['id'])
            if identity in identities:
                raise ValueError(f'Duplicate question identity: {identity}')
            identities.add(identity)
            counts[unit['skill']] += 1
            parts[f"{unit['skill']}/{unit['part']}"] += 1
            refs.update(unit.get('images', []))
            if unit.get('audio'):
                refs.add(unit['audio'])
            for group in unit.get('groups', []):
                points += sum(len(q['answers']) if group['kind'] == 'multiple' else 1 for q in group['questions'])
    files = [(path, f'packs/{path.name}') for path in packs]
    for name in sorted(refs):
        if not re.fullmatch(r'[a-f0-9]{64}\.(mp3|wav|ogg|webm|png|jpg|webp)', name):
            raise ValueError(f'Invalid media reference: {name}')
        path = source / 'media' / name
        digest = hashlib.sha256()
        with path.open('rb') as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b''):
                digest.update(chunk)
        if digest.hexdigest() != name.split('.')[0] or path.stat().st_size > 128 * 1024**2:
            raise ValueError(f'Damaged or oversized media: {name}')
        files.append((path, f'media/{name}'))
    for name in ('README.md', 'quality-audit.json'):
        if (source / name).is_file():
            files.append((source / name, name))
    manifest = dict(version=1, packs=len(packs), counts=dict(counts), parts=dict(parts),
                    attachments=len(refs), answerPoints=points, exams=exams)
    encoded = json.dumps(manifest, ensure_ascii=False, indent=2).encode('utf8')
    total = sum(p.stat().st_size for p, _ in files) + len(encoded)
    if total > 1024**3:
        raise ValueError('Archive contents exceed the 1 GiB importer limit')
    output.parent.mkdir(parents=True, exist_ok=True)
    # Exclusive creation prevents accidentally replacing the user's sole archive.
    with zipfile.ZipFile(output, 'x', zipfile.ZIP_DEFLATED, compresslevel=1) as archive:
        for path, name in files:
            archive.write(path, name)
        archive.writestr('archive-manifest.json', encoded)
    with zipfile.ZipFile(output) as archive:
        if archive.testzip() is not None:
            raise ValueError('ZIP checksum verification failed')
    return manifest


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--source', required=True, type=pathlib.Path)
    parser.add_argument('--output', required=True, type=pathlib.Path)
    args = parser.parse_args()
    print(json.dumps(package(args.source.resolve(), args.output.resolve()), ensure_ascii=False, indent=2))
