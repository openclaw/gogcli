function startsHtmlTag(value, index) {
  const next = value[index + 1]?.toLowerCase();
  return next === "/" || next === "!" || next === "?" || (next >= "a" && next <= "z");
}

export function stripHtmlTags(value) {
  let text = "";
  let insideTag = false;
  let quote = "";

  for (let index = 0; index < value.length; index += 1) {
    const character = value[index];
    if (!insideTag) {
      if (character === "<" && startsHtmlTag(value, index)) {
        insideTag = true;
      } else {
        text += character;
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
  return text;
}
