import importlib.util
import pathlib
import tempfile
import json
import unittest

spec = importlib.util.spec_from_file_location("converter", pathlib.Path(__file__).resolve().parents[1] / "scripts/convert_question_banks.py")
converter = importlib.util.module_from_spec(spec)
spec.loader.exec_module(converter)


def text(value):
    return {"type": "text", "text": value}


def element(tag, *children):
    return {"type": "element", "tag": tag, "children": list(children)}


class ConverterTests(unittest.TestCase):
    def source(self):
        return {"examId": "test", "meta": {"title": "Library"},
                "passageBlocks": [{"nodes": [element("p", text("Open on Monday.")), element("p", text("Closed on Sunday."))]}],
                "questionItems": [{"questionId": "q1"}], "answerKey": {"q1": "Monday"},
                "questionGroups": [{"groupId": "g1", "kind": "sentence_completion", "questionIds": ["q1"], "contentNodes": [
                    element("p", text("ONE WORD ONLY")), element("p", text("The library opens on "), {"type": "textInput", "questionId": "q1"}, text("."))]}]}

    def test_preserves_short_passages_and_question(self):
        unit = converter.convert_reading(self.source())
        self.assertEqual([p["text"] for p in unit["passages"]], ["Open on Monday.", "Closed on Sunday."])
        self.assertEqual(unit["groups"][0]["questions"][0]["answers"], ["Monday"])
        self.assertEqual(unit["groups"][0]["maxWords"], 1)
        self.assertIn("The library opens on", unit["groups"][0]["questions"][0]["text"])

    def test_ignores_executable_markup(self):
        self.assertEqual(converter.plain(element("div", element("script", text("alert(1)")), text("safe"))).strip(), "safe")

    def test_rejects_ambiguous_answers_and_missing_questions(self):
        source = self.source(); source["answerKey"]["q1"] = "(the) Monday"
        with self.assertRaises(ValueError): converter.convert_reading(source)
        source = self.source(); source["questionItems"].append({"questionId": "q2"})
        with self.assertRaises(ValueError): converter.convert_reading(source)

    def test_rejects_required_visuals(self):
        source = self.source(); source["questionGroups"][0]["contentNodes"].append(element("img"))
        with self.assertRaises(ValueError): converter.convert_reading(source)

    def test_preserves_inline_word_boundaries(self):
        self.assertEqual(converter.plain(element("p", text("well-"), element("strong", text("known")), text("."))).strip(), "well-known.")

    def test_preserves_global_numbers_and_years(self):
        source=self.source();source['questionItems'][0]['displayNumber']='14'
        source['questionGroups'][0]['contentNodes'][1]['children'][0]['text']='1400 was the year of '
        unit=converter.convert_reading(source);question=unit['groups'][0]['questions'][0]
        self.assertEqual(question['label'],'14');self.assertEqual(question['id'],'q14')
        self.assertTrue(question['text'].startswith('1400'))
        self.assertIn('[14]',question['text'])

    def test_table_spans_preserve_row_context(self):
        first=element('td',text('Group A'));first['attrs']={'rowspan':'2'}
        table=element('table',element('tr',first,element('td',text('One'))),element('tr',element('td',text('Two'))))
        self.assertEqual(converter.table_grid(table),[['Group A','One'],['Group A','Two']])

    def matching_table_source(self):
        def radio(value):
            return {"type": "choiceInput", "inputType": "radio", "questionId": "q1", "questionIds": ["q1"], "value": value}
        row = element("tr", element("td", element("strong", text("14")), text(" a domestic situation")),
                      element("td", radio("A")), element("td", radio("B")), element("td", radio("C")))
        return {"examId": "m", "meta": {"title": "Match"},
                "passageBlocks": [{"nodes": [element("p", text("Para one text.")), element("p", text("Para two text."))]}],
                "questionItems": [{"questionId": "q1", "displayNumber": "14"}], "answerKey": {"q1": "A"},
                "questionGroups": [{"groupId": "g1", "kind": "table_completion", "questionIds": ["q1"], "contentNodes": [
                    element("p", text("Which paragraph contains the following information?")), element("table", element("tbody", row))]}]}

    def test_matching_table_bare_radio_options(self):
        # Paragraph-matching tables encode options as bare <td> radios (no <label>/<select>);
        # the group's shared option bank must still be extracted so the first question is not rejected.
        unit = converter.convert_reading(self.matching_table_source())
        question = unit["groups"][0]["questions"][0]
        self.assertEqual(question["id"], "q14")
        self.assertEqual(question["answers"], ["a"])
        self.assertIn("a", {option["id"] for option in question["options"]})
        self.assertGreaterEqual(len(question["options"]), 2)

    def mislabeled_tfng_source(self):
        def radio(value):
            return {"type": "choiceInput", "inputType": "radio", "questionId": "q1", "questionIds": ["q1"], "value": value}
        statement = element("p", element("strong", text("1")), text(" A statement to verify."),
                            radio("TRUE"), radio("FALSE"), radio("NOT GIVEN"))
        return {"examId": "tf", "meta": {"title": "TF"},
                "passageBlocks": [{"nodes": [element("p", text("Passage sentence one.")), element("p", text("Passage sentence two."))]}],
                "questionItems": [{"questionId": "q1", "displayNumber": "1"}], "answerKey": {"q1": "TRUE"},
                "questionGroups": [{"groupId": "g1", "kind": "yes_no_not_given", "questionIds": ["q1"], "contentNodes": [
                    element("p", text("Do the following statements agree with the information?")), statement]}]}

    def test_mislabeled_yes_no_group_with_true_false_values(self):
        # Some sources tag a TRUE/FALSE/NOT GIVEN group as yes_no_not_given. When the controls
        # (and answers) are a clean TRUE/FALSE/NOT GIVEN set it is a safe mislabel, not a mix.
        unit = converter.convert_reading(self.mislabeled_tfng_source())
        question = unit["groups"][0]["questions"][0]
        self.assertEqual(question["answers"], ["true"])
        self.assertEqual({option["id"] for option in question["options"]}, {"true", "false", "not_given"})

    def test_rejects_truly_mixed_yes_no_controls(self):
        # A genuine mix of YES/NO and TRUE/FALSE controls stays rejected.
        source = self.mislabeled_tfng_source()
        source["questionGroups"][0]["contentNodes"][1]["children"].append(
            {"type": "choiceInput", "inputType": "radio", "questionId": "q1", "questionIds": ["q1"], "value": "YES"})
        with self.assertRaises(ValueError):
            converter.convert_reading(source)

    def listening_section(self):
        return {"id":"section-1","section_number":1,"title":"Booking","transcript":"Monday.","question_groups":[{
            "id":"g1","question_type":"sentence-completion","instructions":"ONE WORD ONLY","questions":[{
                "id":"q1","question_order":1,"text":"Day: ____","answer":"Monday","accepted_answers":["Monday","monday"]}]}]}

    def test_listening_alternatives_and_missing_diagram(self):
        section=self.listening_section()
        unit=converter.convert_listening_section(section,"a"*64+".wav")
        self.assertEqual(len(unit["groups"][0]["questions"][0]["answers"]),1)
        section["question_groups"][0]["instructions"]="Look at the map of Terminal 3."
        with self.assertRaises(ValueError): converter.convert_listening_section(section,"a"*64+".wav")

    def test_mislabeled_wave_and_missing_audio(self):
        with tempfile.TemporaryDirectory() as directory:
            root=pathlib.Path(directory);out=root/"output";out.mkdir()
            section=self.listening_section()
            (root/"manifest.json").write_text(json.dumps({"sections":[section]}),encoding="utf-8")
            self.assertEqual(converter.convert_listening(root,out),(0,1))
            (root/"section-1.mp3").write_bytes(b"RIFFxxxxWAVEtest")
            self.assertEqual(converter.convert_listening(root,out),(1,0))
            pack=json.loads((out/"listening-starter.json").read_text(encoding="utf-8"))
            self.assertTrue(pack["units"][0]["audio"].endswith(".wav"))

    def test_writing_task1_pairs_image_and_skips_imageless(self):
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory); source = root / "src"; source.mkdir(); out = root / "out"; out.mkdir()
            # A real JPEG whose declared url wrongly says .png — the content sniff must still win.
            (source / "t.jpeg").write_bytes(bytes.fromhex("ffd8ffe000104a46494600010100000100010000ffd9"))
            (source / "t.json").write_text(json.dumps({"tasks": [
                {"id": "a", "task_number": 1, "title": "Chart", "prompt": "Describe the chart.", "image_url": "./t.png"},
                {"id": "b", "task_number": 2, "title": "Essay", "prompt": "Discuss both views."},
                {"id": "d", "task_number": 1, "title": "General Training Task 1: write a letter", "prompt": "Write a letter.", "image_url": "./t.png"}]}), encoding="utf-8")
            (source / "noimg.json").write_text(json.dumps({"tasks": [
                {"id": "c", "task_number": 1, "title": "NoChart", "prompt": "x", "image_url": ""}]}), encoding="utf-8")
            units = {u["id"]: u for u in converter.convert_writing(source, out)}
            self.assertEqual(units["a"]["part"], "Task-1-Academic")
            self.assertEqual(units["d"]["part"], "Task-1-General")
            self.assertTrue(units["a"]["images"][0].endswith(".jpg"))
            self.assertTrue((out / units["a"]["images"][0]).is_file())
            self.assertEqual(units["b"]["part"], "Task-2")
            # An image-based Task 1 with no real figure is skipped (the user adds a complete one via
            # in-app authoring with its screenshot); it is never emitted as a text-only guess.
            self.assertNotIn("c", units)

    def test_liz_writing_extracts_blockquote_task2_prompts(self):
        html = ("<html><body><p>intro, not a question</p>"
                "<blockquote><p>Context about schools.<br/>To what extent do you agree? (Reported 2017)</p></blockquote>"
                "<blockquote><p>A teacher&#8217;s role matters.</p><p>Discuss both views and give your opinion.</p></blockquote>"
                "</body></html>")
        with tempfile.TemporaryDirectory() as directory:
            path = pathlib.Path(directory) / "liz.html"
            path.write_text(html, encoding="utf-8")
            units = converter.convert_liz_writing(path)
        self.assertEqual(len(units), 2)
        self.assertTrue(all(u["skill"] == "writing" and u["part"] == "Task-2" for u in units))
        self.assertIn("Context about schools.", units[0]["prompt"])
        self.assertIn("To what extent do you agree", units[0]["prompt"])
        self.assertIn("teacher’s role", units[1]["prompt"])
        self.assertIn("Discuss both views", units[1]["prompt"])

    def test_listening_resources_pair_audio_strip_timestamps_no_groups(self):
        # Found exam audio without digital questions imports as a listen-only resource:
        # audio + machine transcript, no question groups (the learner answers on paper).
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory); (root / "mp3").mkdir(); out = root / "out"; out.mkdir()
            (root / "Test1-Part1.lrc").write_text("[00:00.35]Welcome to the test.\n[00:02.59]Now turn to part one.\n", encoding="utf-8")
            (root / "mp3" / "Test1-Part1.mp3").write_bytes(b"\xff\xfb\x90\x00audio-bytes")
            (root / "Test1-Part2.lrc").write_text("[00:00.00]Section two has no paired audio.\n", encoding="utf-8")
            units = converter.convert_listening_resources(root, out)
            self.assertEqual(len(units), 1)  # the .lrc without a paired mp3 is skipped
            unit = units[0]
            self.assertEqual(unit["skill"], "listening")
            self.assertEqual(unit["part"], "1")
            self.assertNotIn("groups", unit)  # no questions: a listen-only resource, not auto-graded
            self.assertTrue(unit["audio"].endswith(".mp3"))
            self.assertTrue((out / unit["audio"]).is_file())
            self.assertIn("Welcome to the test.", unit["transcript"])
            self.assertNotIn("[00:00", unit["transcript"])  # lrc timestamps stripped


if __name__ == "__main__":
    unittest.main()
