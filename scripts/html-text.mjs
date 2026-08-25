function startsHtmlTag(value, index) {
  const next = value[index + 1]?.toLowerCase();
  return next === "/" || next === "!" || next === "?" || (next >= "a" && next <= "z");
}

export function stripHtmlTags(value) {
  const text = [];
  let insideTag = false;
  let quote = "";

  for (let index = 0; index < value.length; index += 1) {
    const character = value[index];
    if (!insideTag) {
      if (character === "<" && startsHtmlTag(value, index)) {
        while (text.at(-1) === "<") text.pop();
        insideTag = true;
      } else {
        text.push(character);
      }
      continue;
    }
    if (quote) {
      if (character === quote) quote = "";
    } else if (character === '"' || character === "'") {
      quote = character;
    } else if (character === ">") {
      insideTag = false;
    }
  }
  return text.join("");
}
