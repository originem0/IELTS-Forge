"""Convert local source data; never import or execute third-party application code.

Outputs stay in the user's ignored question-banks directory. Schema extraction is
not an endorsement of the source's answers or redistribution rights.
"""
import argparse
import copy
import hashlib
import json
import pathlib
import re
from html.parser import HTMLParser


def walk(nodes):
    if isinstance(nodes, list):
        for node in nodes:
            yield from walk(node)
    elif isinstance(nodes, dict):
        yield nodes
        yield from walk(nodes.get("children", []))


def compact(text):
    return re.sub(r"[ \t]+", " ", re.sub(r"\s*\n\s*", "\n", text)).strip()


def plain(nodes, choices=False, omit_questions=False):
    if isinstance(nodes, list):
        return "".join(plain(node, choices, omit_questions) for node in nodes)
    kind, tag = nodes.get("type"), nodes.get("tag")
    if kind == "text":
        return nodes.get("text", "")
    if kind in ("textInput", "dropzone", "select"):
        return f" [{nodes.get('questionId', '').removeprefix('q')}] ____ "
    if kind in ("choiceInput", "optionChip"):
        return ""
    if tag in ("script", "style", "button", "svg", "img", "table"):
        return ""
    if not choices and tag == "label" and any(n.get("type") == "choiceInput" for n in walk(nodes)):
        return ""
    attrs = nodes.get("attrs", {})
    if omit_questions and "question-item" in attrs.get("class", ""):
        return ""
    result = plain(nodes.get("children", []), choices, omit_questions)
    return result + ("\n" if tag in ("p", "div", "li", "h2", "h3", "h4", "br", "tr") else "")


def question_ids(node):
    found = set()
    for child in walk(node):
        if child.get("questionId"):
            found.add(child["questionId"])
        found.update(child.get("questionIds", []))
    return found


def option_id(value):
    return re.sub(r"[^A-Za-z0-9_-]", "_", value.strip()).casefold()


def group_options(nodes, qid=None):
    options = {}
    for node in walk(nodes):
        if node.get("type") == "select" and (qid is None or node.get("questionId") == qid):
            for option in node.get("options", []):
                if option.get("value"):
                    options[option_id(option["value"])] = option["label"]
        elif node.get("type") == "optionChip":
            options[option_id(node["value"])] = node["label"]
        elif node.get("tag") == "label":
            controls = [n for n in walk(node) if n.get("type") == "choiceInput" and (qid is None or qid in question_ids(n))]
            for control in controls:
                options[option_id(control["value"])] = compact(plain(node, choices=True)) or control["value"]
    # Matching/paragraph tables encode options as bare <td> radios with no <label>/<select>;
    # fall back to the control's own value so the shared option bank is not lost. Richer
    # label text captured above wins via setdefault.
    for node in walk(nodes):
        if node.get("type") == "choiceInput" and (qid is None or node.get("questionId") == qid or qid in node.get("questionIds", [])):
            options.setdefault(option_id(node["value"]), node["value"])
    return [{"id": key, "text": value} for key, value in options.items()]


def table_grid(table):
    if sum(node.get("tag") == "table" for node in walk(table)) > 1:
        raise ValueError("nested table requires manual conversion")
    grid = {}
    rows = [node for node in walk(table) if node.get("tag") == "tr"]
    for row_index, row in enumerate(rows):
        column = 0
        for cell in row.get("children", []):
            if cell.get("tag") not in ("td", "th"):
                continue
            while (row_index, column) in grid:
                column += 1
            span = int(cell.get("attrs", {}).get("colspan", 1))
            depth = int(cell.get("attrs", {}).get("rowspan", 1))
            if span < 1 or depth < 1 or column + span > 20 or row_index + depth > 100:
                raise ValueError("table span out of range")
            value = compact(plain(cell))
            for y in range(row_index, row_index + depth):
                for x in range(column, column + span):
                    grid[y, x] = value if x == column else ""
            column += span
    if not grid:
        raise ValueError("empty table")
    width = max(x for _, x in grid) + 1
    return [[grid.get((y, x), "") for x in range(width)] for y in range(max(y for y, _ in grid)+1)]


def convert_reading(document):
    document = copy.deepcopy(document)
    labels = {q["questionId"]: str(q.get("displayNumber", q["questionId"].removeprefix("q"))) for q in document["questionItems"]}
    if len(labels) != len(document["questionItems"]) or any(not label.isdecimal() for label in labels.values()) or len(set(labels.values())) != len(labels):
        raise ValueError("ambiguous display numbering")
    renumber = {qid: f"q{label}" for qid, label in labels.items()}
    def rewrite(value):
        if isinstance(value, list):
            for child in value: rewrite(child)
        elif isinstance(value, dict):
            if value.get("questionId") in renumber: value["questionId"] = renumber[value["questionId"]]
            if "questionIds" in value: value["questionIds"] = [renumber.get(qid, qid) for qid in value["questionIds"]]
            for child in value.values(): rewrite(child)
    rewrite(document)
    document["answerKey"] = {renumber.get(qid, qid): value for qid, value in document["answerKey"].items()}
    supported = {"matching", "single_choice", "multi_choice", "true_false_not_given", "yes_no_not_given", "sentence_completion", "short_answer", "summary_completion", "table_completion", "classification"}
    if any(group["kind"] not in supported for group in document["questionGroups"]):
        raise ValueError("contains table/diagram requiring a dedicated layout")
    passage_nodes = [node for block in document["passageBlocks"] for node in block.get("nodes", [])]
    if any(n.get("tag") in ("img", "svg", "table") for n in walk(passage_nodes)):
        raise ValueError("passage depends on image/table")
    paragraphs, headings = [], {}
    for node in walk(passage_nodes):
        if node.get("type") == "dropzone" and node.get("paragraph"):
            headings[node["questionId"]] = node["paragraph"]
        if node.get("tag") != "p":
            continue
        text = compact(plain(node))
        if not text or text.startswith("You should spend"):
            continue
        first = next((n for n in node.get("children", []) if n.get("type") != "text" or n.get("text", "").strip()), {})
        label = compact(plain(first))
        paragraph_id = label if first.get("tag") in ("strong", "b") and re.fullmatch("[A-Z]", label) else f"P{len(paragraphs)+1}"
        if paragraph_id == label:
            text = text[len(label):].strip()
        paragraphs.append({"id": paragraph_id, "text": text})
    if not paragraphs or len({p['id'] for p in paragraphs}) != len(paragraphs):
        raise ValueError("missing or ambiguous paragraphs")
    groups = []
    source_qids = {q["questionId"] for q in document["questionItems"]}
    converted = set()
    for original in document["questionGroups"]:
        nodes = original["contentNodes"]
        all_nodes = list(walk(nodes))
        kind = original["kind"]
        if any(n.get("tag") in ("img", "svg") for n in all_nodes):
            raise ValueError("question depends on image/table")
        tables = [n for n in all_nodes if n.get("tag") == "table"]
        if len(tables) > 1:
            raise ValueError("multiple tables require manual association")
        if kind == "yes_no_not_given":
            control_values = {n.get("value") for n in all_nodes if n.get("type") == "choiceInput"}
            # A clean TRUE/FALSE/NOT GIVEN control set is a source that mislabeled a TFNG group;
            # both map to single choice, so keep it. Reject only a genuine YES/NO + TRUE/FALSE mix.
            if control_values & {"TRUE", "FALSE"} and not control_values <= {"TRUE", "FALSE", "NOT GIVEN"}:
                raise ValueError("YES/NO group mixes TRUE/FALSE and YES/NO controls")
        mapped = "matching" if kind in ("matching", "classification") else "multiple" if kind == "multi_choice" else "single" if kind in ("single_choice", "true_false_not_given", "yes_no_not_given") else "text"
        if kind == "table_completion" and any(n.get("type") == "choiceInput" and n.get("inputType") == "radio" for n in all_nodes):
            mapped = "single"
        if mapped == "text" and any(n.get("type") in ("select", "dropzone") for n in all_nodes) and group_options(nodes):
            mapped = "matching"
        instruction = compact(plain(nodes, omit_questions=True))
        instruction = "\n".join(line for line in instruction.splitlines() if not re.search(r"\bdrag\b.*\bdrop\b|^Drag a heading", line, re.I))
        if mapped == "text":
            match = re.search(r"(?:NO MORE THAN\s+)?(ONE|TWO|THREE|FOUR|\d+)\s+WORDS?(?:\s+AND/OR\s+A\s+NUMBER)?", instruction, re.I)
            number_only = bool(re.search(r"A NUMBER ONLY|ONE NUMBER ONLY", instruction, re.I))
            if not match and not number_only:
                raise ValueError("text answer has no reliable word limit")
        else:
            match, number_only = None, False
        group = {"id": original["groupId"], "kind": mapped, "instruction": instruction, "questions": []}
        if tables:
            group["table"] = table_grid(tables[0])
        if match:
            term = match[1].upper()
            group["maxWords"] = {"ONE": 1, "TWO": 2, "THREE": 3, "FOUR": 4}.get(term, int(term) if term.isdigit() else 0)
            group["allowNumber"] = "NUMBER" in match[0].upper()
        if number_only:
            group["numberOnly"] = True
        qids = original["questionIds"]
        if mapped == "multiple":
            controls = [n for n in all_nodes if n.get("type") == "choiceInput"]
            if not controls or any(set(c.get("questionIds", [])) != set(qids) for c in controls):
                raise ValueError("multiple-choice groups need manual separation")
        options = group_options(nodes) if mapped in ("matching", "multiple") else []
        if options:
            group["options"] = options
        explanation_items = {item["questionId"]: item["text"] for item in (original.get("explanationSection") or {}).get("items", [])}
        for qid in ([qids[0]] if mapped == "multiple" else qids):
            candidates = []
            for node in all_nodes:
                if node.get("type") == "element" and question_ids(node) == {qid}:
                    text = compact(plain(node))
                    if len(text) > 12:
                        candidates.append(text)
            if qid in headings:
                prompt = f"Paragraph {headings[qid]}"
            elif candidates:
                prompt = min(candidates, key=len)
            elif kind == "summary_completion" or tables:
                prompt = f"Blank {qid.removeprefix('q')}"
            elif mapped == "multiple":
                prompt = "Select the required options."
            else:
                raise ValueError(f"cannot reliably identify prompt for {qid}")
            prompt = re.sub(rf"^{re.escape(qid.removeprefix('q'))}[.)]?\s+", "", prompt)
            if mapped != "text":
                prompt = re.sub(r"\[\d+\]\s*_+\s*", "", prompt)
                prompt = re.sub(rf"^{re.escape(qid.removeprefix('q'))}[.)]?\s+", "", prompt)
            else:
                prompt = re.sub(r"\b(\d+)\s+\[\1\]", r"[\1]", prompt)
            if not prompt.strip():
                raise ValueError(f"empty question text for {qid}")
            expected_ids = qids if mapped == "multiple" else [qid]
            answers = []
            for expected_id in expected_ids:
                value = document["answerKey"].get(expected_id)
                if not isinstance(value, str) or not value.strip():
                    raise ValueError(f"missing answer for {expected_id}")
                # Slash/parenthesized keys can encode alternatives or optional words;
                # do not silently guess which interpretation the source intended.
                if mapped == "text" and any(char in value for char in "/()"):
                    raise ValueError(f"ambiguous alternative answer for {expected_id}")
                answers.append(value if mapped == "text" else option_id(value))
            question = {"id": qid, "label": "–".join(q.removeprefix("q") for q in (qids[0], qids[-1])) if mapped == "multiple" else qid.removeprefix("q"), "text": prompt, "answers": answers}
            if mapped == "single":
                question["options"] = group_options(nodes, qid)
            available = {opt["id"] for opt in question.get("options", options)}
            if mapped != "text" and (len(available) < 2 or not set(answers).issubset(available)):
                raise ValueError(f"incomplete options for {qid}")
            if qid in explanation_items:
                question["explanation"] = explanation_items[qid]
            if qid in headings and headings[qid] in {p['id'] for p in paragraphs}:
                question["evidence"] = headings[qid]
            group["questions"].append(question)
            converted.update(expected_ids)
        if kind == "summary_completion":
            group["context"] = compact(plain(nodes))
            group["instruction"] = match[0] if match else "Complete the summary."
        elif mapped != "multiple":
            # Remove question sentences duplicated in the source's instruction tree.
            for question in group["questions"]:
                group["instruction"] = group["instruction"].replace(question["text"], "")
            group["instruction"] = compact(group["instruction"])
            group["instruction"] = "\n".join(line for line in group["instruction"].splitlines() if not re.fullmatch(r"\d+[.)]?", line.strip()))
        groups.append(group)
    if converted != source_qids:
        raise ValueError("question coverage differs from source")
    return {"id": document["examId"], "skill": "reading", "part": "academic", "title": document["meta"]["title"], "prompt": "Read the passage and answer the questions. Follow each group's answer instructions.", "minutes": 20, "passages": paragraphs, "groups": groups}


def convert_speaking(path):
    source = path.read_text(encoding="utf-8")
    match = re.search(r"var G\s*=\s*", source)
    if not match:
        raise ValueError("speaking source has no recognized JSON array")
    data = json.JSONDecoder().raw_decode(source[match.end():])[0]
    units = []
    for group in data:
        part, title = group["part"], group["title"]
        if part not in ("p1", "p2", "p3"):
            raise ValueError("unknown speaking part")
        prompts = [group["cue"].strip()] if part == "p2" else [line["t"].strip() for line in group["lines"]]
        for index, prompt in enumerate(prompts):
            if not prompt:
                raise ValueError("empty speaking prompt")
            identity = hashlib.sha256(f"{part}\n{title}\n{prompt}".encode()).hexdigest()[:20]
            units.append({"id": f"speaking-{identity}", "skill": "speaking", "part": part, "title": title if part == "p2" else f"{title} · {index+1}", "prompt": prompt, "minutes": 2 if part == "p2" else 0, "_topic": title})
    for unit in units:
        if unit["part"] in ("p2", "p3"):
            unit["related"] = [other["id"] for other in units if other["_topic"] == unit["_topic"] and {unit["part"], other["part"]} == {"p2", "p3"}]
    for unit in units:
        unit.pop("_topic")
    # Remove identical source questions without using acquisition timestamps.
    return list({unit["id"]: unit for unit in units}.values())


def image_extension(data):
    # Extensions the launcher's media validator accepts (jpg, not jpeg; no gif).
    if data[:3] == b"\xff\xd8\xff":
        return ".jpg"
    if data[:8] == b"\x89PNG\r\n\x1a\n":
        return ".png"
    if data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        return ".webp"
    raise ValueError("unsupported image format")


def convert_writing(root, output):
    units = []
    for path in sorted(root.rglob("*.json")):
        document = json.loads(path.read_text(encoding="utf-8"))
        for task in document.get("tasks", []):
            number = task.get("task_number")
            if number == 2:
                units.append({"id": task["id"], "skill": "writing", "part": "Task-2", "title": task["title"], "prompt": task["prompt"], "minutes": 40})
            elif number == 1:
                hint = f"{document.get('title', '')} {task.get('title', '')} {task.get('task_type', '')}"
                general = (re.search(r"\bgeneral[\s_-]+training\b|\btask[\s_-]*1[\s_-]+general\b|\bletter\b", hint, re.I)
                           or str(task.get('task_type', '')).lower() == 'general'
                           or re.search(r"\bwrite\s+a\s+letter\b", task.get('prompt', ''), re.I))
                if general:
                    # A letter is complete as text. Do not reject it for a missing
                    # chart or accidentally attach another task's sibling image.
                    units.append({"id": task["id"], "skill": "writing", "part": "Task-1-General", "title": task["title"], "prompt": task["prompt"], "minutes": 20})
                    continue
                # Academic Task 1 needs the real figure; a description never substitutes for it. The
                # declared image_url is unreliable (wrong or empty extension), so resolve by the
                # sibling sharing the document stem and confirm the type by content, not by name.
                candidates = [path.with_suffix(ext) for ext in (".jpeg", ".jpg", ".png", ".webp", ".gif")]
                if task.get("image_url"):
                    candidates.insert(0, path.parent / task["image_url"])
                image = next((candidate for candidate in candidates if candidate.is_file()), None)
                if image is None:
                    continue
                data = image.read_bytes()
                try:
                    extension = image_extension(data)
                except ValueError:
                    continue  # unsupported image format — skip rather than emit an invalid reference
                media_id = hashlib.sha256(data).hexdigest() + extension
                (output / media_id).write_bytes(data)
                units.append({"id": task["id"], "skill": "writing", "part": "Task-1-Academic", "title": task["title"], "prompt": task["prompt"], "minutes": 20, "images": [media_id]})
    return units


class _LizEssayParser(HTMLParser):
    """Collect each top-level <blockquote> as one Task 2 prompt; HTMLParser decodes entities."""
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.depth = 0
        self.buffer = []
        self.prompts = []

    def handle_starttag(self, tag, attrs):
        if tag == "blockquote":
            if self.depth == 0:
                self.buffer = []
            self.depth += 1
        elif tag in ("br", "p") and self.depth:
            self.buffer.append("\n")

    def handle_endtag(self, tag):
        if tag == "blockquote" and self.depth:
            self.depth -= 1
            if self.depth == 0:
                prompt = compact("".join(self.buffer))
                if prompt:
                    self.prompts.append(prompt)

    def handle_data(self, data):
        if self.depth:
            self.buffer.append(data)


def convert_liz_writing(path):
    parser = _LizEssayParser()
    parser.feed(path.read_text(encoding="utf-8"))
    units = []
    for prompt in parser.prompts:
        identity = hashlib.sha256(prompt.encode()).hexdigest()[:16]
        title = prompt.split("\n")[0].split(". ")[0][:80]
        units.append({"id": f"writing-liz-{identity}", "skill": "writing", "part": "Task-2", "title": title, "prompt": prompt, "minutes": 40})
    # Collapse identical prompts without relying on acquisition order.
    return list({unit["id"]: unit for unit in units}.values())


def write_pack(output, filename, title, source, units):
    pack = {"version": 1, "title": title, "source": source, "units": units}
    (output / filename).write_text(json.dumps(pack, ensure_ascii=False, indent=2), encoding="utf-8")


def convert_listening_section(section, audio_id):
    if section.get("section_number") not in (1, 2, 3, 4) or not section.get("transcript", "").strip():
        raise ValueError("missing listening part or transcript")
    groups = []
    for original in section["question_groups"]:
        instruction = original["instructions"]
        if re.search(r"\b(map|diagram|plan of)\b", instruction, re.I):
            raise ValueError("required map/diagram is not present in this source")
        kind = {"sentence-completion": "text", "multiple-choice": "single"}.get(original["question_type"])
        if kind is None:
            raise ValueError("unsupported listening question type")
        group = {"id": original["id"], "kind": kind, "instruction": instruction, "questions": []}
        if kind == "text":
            match = re.search(r"(?:NO MORE THAN\s+)?(ONE|TWO|THREE|\d+)\s+WORDS?(?:\s+AND/OR\s+A\s+NUMBER)?", instruction, re.I)
            if not match:
                raise ValueError("missing reliable listening answer limit")
            term = match[1].upper()
            group["maxWords"] = {"ONE": 1, "TWO": 2, "THREE": 3}.get(term, int(term) if term.isdigit() else 0)
            group["allowNumber"] = "NUMBER" in match[0].upper()
        for original_q in original["questions"]:
            answer = original_q["answer"]
            if not isinstance(answer, str) or not answer.strip():
                raise ValueError("invalid listening answer")
            alternatives = original_q.get("accepted_answers") or [answer]
            if not all(isinstance(value, str) and value.strip() for value in alternatives):
                raise ValueError("invalid listening alternatives")
            alternatives = list({value.strip().casefold(): value.strip() for value in [answer, *alternatives]}.values())
            question = {"id": original_q["id"], "label": str(original_q["question_order"]), "text": original_q["text"], "answers": alternatives}
            if kind == "single":
                options = []
                for option in original_q["options"]:
                    match = re.match(r"^([A-Z])(?:[.)]\s*|$)", option)
                    if not match:
                        raise ValueError("invalid listening option label")
                    options.append({"id": match[1], "text": option})
                question["options"] = options
                if not set(alternatives).issubset({option["id"] for option in options}):
                    raise ValueError("listening answer not among options")
            group["questions"].append(question)
        groups.append(group)
    return {"id": section["id"], "skill": "listening", "part": str(section["section_number"]), "title": section["title"], "prompt": "Listen to the recording and answer the questions. Follow each group's answer instructions.", "minutes": 0, "audio": audio_id, "transcript": section["transcript"], "groups": groups}


def convert_listening(root, output):
    units, rejected = [], []
    for path in sorted(root.rglob("manifest.json")):
        document = json.loads(path.read_text(encoding="utf-8"))
        for section in document.get("sections", []):
            audio = path.parent / f"section-{section['section_number']}.mp3"
            try:
                if not audio.is_file():
                    raise ValueError("missing audio file")
                raw = audio.read_bytes()
                if raw.startswith(b"RIFF") and raw[8:12] == b"WAVE":
                    extension = ".wav"
                elif raw and (raw.startswith(b"ID3") or raw[0] == 0xff):
                    extension = ".mp3"
                else:
                    raise ValueError("unrecognized audio container")
                # One source labels a WAVE recording .mp3. Preserve its bytes,
                # but publish the true container type for reliable offline playback.
                media_id = hashlib.sha256(raw).hexdigest() + extension
                unit = convert_listening_section(section, media_id)
                (output / media_id).write_bytes(raw)
                units.append(unit)
            except (KeyError, ValueError, TypeError) as error:
                rejected.append({"source": path.parent.name, "section": section.get("section_number"), "reason": str(error)})
    if units:
        write_pack(output, "listening-starter.json", f"听力生成练习 · {len(units)} 段", {"name": "LuchoBazz/ielts-ai-dataset", "url": "https://github.com/LuchoBazz/ielts-ai-dataset", "status": "generated", "license": "CC BY 4.0 (source declaration)"}, units)
    (output / "listening-conversion-audit.json").write_text(json.dumps({"included": [unit["id"] for unit in units], "rejected": rejected, "note": "Individual sections only; no complete verified test or timestamp alignment is asserted."}, ensure_ascii=False, indent=2), encoding="utf-8")
    return len(units), len(rejected)


LRC_TIMESTAMP = re.compile(r"^\s*(?:\[\d{1,2}:\d{2}(?:[.:]\d{1,3})?\]\s*)+")


def lrc_transcript(text):
    # Strip leading [mm:ss.xx] timestamps; keep the spoken lines as the transcript body.
    lines = [LRC_TIMESTAMP.sub("", line).strip() for line in text.splitlines()]
    return "\n".join(line for line in lines if line)


def convert_listening_resources(root, output):
    """Found exam audio without digital questions: import as listen-only resources.

    Each paired Test*-Part*.lrc + mp3/Test*-Part*.mp3 becomes one listening unit with audio
    and the machine transcript but NO question groups — the learner answers from the paper
    book and nothing here is auto-graded. Transcripts are auto-generated and may contain errors.
    """
    units = []
    for lrc in sorted(root.glob("*.lrc")):
        match = re.fullmatch(r"Test(\d+)-Part([1-4])", lrc.stem)
        if not match:
            continue
        audio = root / "mp3" / f"{lrc.stem}.mp3"
        if not audio.is_file():
            continue
        raw = audio.read_bytes()
        if not (raw.startswith(b"ID3") or (raw and raw[0] == 0xff)):
            continue  # not an MP3 container; the transcoded files are all MP3
        transcript = lrc_transcript(lrc.read_text(encoding="utf-8"))
        if not transcript:
            continue
        media_id = hashlib.sha256(raw).hexdigest() + ".mp3"
        (output / media_id).write_bytes(raw)
        test_no, part = match[1], match[2]
        units.append({"id": f"camb21-test{test_no}-part{part}", "skill": "listening", "part": part,
                      "title": f"剑桥雅思 21 · Test {test_no} · Part {part}（听音资源）",
                      "prompt": "听录音，题目见纸质书（剑桥雅思 21）。本片仅供听音与核对原文，不在应用内作答或判分；原文为自动转写，可能有误。",
                      "minutes": 0, "audio": media_id, "transcript": transcript})
    return units


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--reading", type=pathlib.Path, help="reading-native directory")
    parser.add_argument("--speaking", type=pathlib.Path, help="source index.html containing var G JSON")
    parser.add_argument("--writing", type=pathlib.Path, help="generated writing datasets directory")
    parser.add_argument("--writing-liz", type=pathlib.Path, help="IELTS Liz essay-questions HTML (local personal use only)")
    parser.add_argument("--listening", type=pathlib.Path, help="generated listening datasets directory")
    parser.add_argument("--listening-resources", type=pathlib.Path, help="found exam audio dir (.lrc files + mp3/ subdir) imported as listen-only resources")
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--limit", type=int, default=12)
    args = parser.parse_args()
    if not (args.reading or args.speaking or args.writing or args.writing_liz or args.listening or args.listening_resources):
        parser.error("select a source directory or file")
    args.output.mkdir(parents=True, exist_ok=True)
    if args.listening:
        included, rejected = convert_listening(args.listening, args.output)
        print(json.dumps({"listening": included, "rejected": rejected}))
    if args.listening_resources:
        units = convert_listening_resources(args.listening_resources, args.output)
        if units:
            write_pack(args.output, "listening-resources-starter.json", f"听力听音资源 · {len(units)} 段（剑桥雅思 21 · 仅本地个人使用）",
                       {"name": "剑桥雅思 21 Cambridge IELTS 21（2026）", "status": "verified", "license": "Cambridge 版权；仅本地个人使用，不随项目分发"}, units)
        print(json.dumps({"listening_resources": len(units)}))
    if args.speaking:
        units = convert_speaking(args.speaking)
        source = {"name": "murph232439/ielts-listening (speaking questions)", "url": "https://github.com/murph232439/ielts-listening", "status": "unverified", "season": "2026-09 to 2026-12 (source label)"}
        for part, subset in [("p1", [u for u in units if u["part"] == "p1"]), ("p2-p3", [u for u in units if u["part"] != "p1"])]:
            write_pack(args.output, f"speaking-{part}-starter.json", f"口语题库 · {part.upper()}", source, subset)
        print(json.dumps({"speaking": len(units)}))
    if args.writing:
        units = convert_writing(args.writing, args.output)
        if units:
            write_pack(args.output, "writing-starter.json", "写作 · 生成练习（含 Task 1 图表）", {"name": "LuchoBazz/ielts-ai-dataset", "url": "https://github.com/LuchoBazz/ielts-ai-dataset", "status": "generated", "license": "CC BY 4.0 (source declaration)"}, units)
        print(json.dumps({"writing": len(units)}))
    if args.writing_liz:
        units = convert_liz_writing(args.writing_liz)
        if units:
            # Separate pack: copyrighted blog source, kept out of the generated LuchoBazz pack so
            # provenance stays honest. Local personal import only — never bundle or publish.
            write_pack(args.output, "writing-liz-starter.json", f"写作 Task 2 · 教育类 {len(units)} 题（ieltsliz.com · 仅本地个人使用）",
                       {"name": "ieltsliz.com education essay questions", "url": "https://ieltsliz.com/", "status": "unverified", "license": "copyrighted source; local personal import only, not for redistribution"}, units)
        print(json.dumps({"writing_liz": len(units)}))
    if not args.reading:
        return
    candidates, rejected = [], []
    for path in sorted((args.reading / "exams").glob("*.json")):
        try:
            unit = convert_reading(json.loads(path.read_text(encoding="utf-8")))
            candidates.append(unit)
        except (KeyError, ValueError, TypeError) as error:
            rejected.append({"file": path.name, "reason": str(error)})
    # Prefer coverage of question interactions before adding more of the same.
    chosen, covered = [], set()
    for unit in candidates:
        kinds = {group["kind"] for group in unit["groups"]}
        if not kinds.issubset(covered) and len(chosen) < args.limit:
            chosen.append(unit); covered.update(kinds)
    chosen += [unit for unit in candidates if unit not in chosen][:max(0, args.limit-len(chosen))]
    if not chosen:
        raise SystemExit("No source units passed conversion checks")
    args.output.mkdir(parents=True, exist_ok=True)
    pack = {"version": 1, "title": f"阅读起步题包 · {len(chosen)} 篇", "source": {"name": "hwttop5/ielts-reading-past-papers", "url": "https://github.com/hwttop5/ielts-reading-past-papers", "status": "unverified"}, "units": chosen}
    (args.output / "reading-starter.json").write_text(json.dumps(pack, ensure_ascii=False, indent=2), encoding="utf-8")
    report = {"selected": [unit["id"] for unit in chosen], "convertedCandidates": len(candidates), "types": sorted(covered), "rejected": rejected, "note": "Structural conversion only; source answer correctness and content rights remain unverified."}
    (args.output / "reading-conversion-audit.json").write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps({"selected": len(chosen), "candidates": len(candidates), "rejected": len(rejected), "types": sorted(covered)}))


if __name__ == "__main__":
    main()
