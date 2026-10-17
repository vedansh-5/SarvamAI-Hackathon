export const CONTACT_EMAIL = "vedansh@summonvans.in";

export const KEYS = {
  SUMMON: "⌥Space",
  ESC: "esc",
  DO_FOR_ME: "⌘↵",
  DO_ALL: "⇧⌘↵",
  RETURN: "↵",
} as const;

export interface RequestFact {
  id: string;
  query: string;
  whatHappens: string;
  kind: "teaching" | "marking" | "reading" | "files";
  group: "show" | "mark" | "do";
  colorToken: "blue" | "teal" | "green" | "violet";
  targetDemo: "guide" | "panel";
  sampleAnswer?: string;
  sourceApp?: string;
}

export const REQUEST_FACTS: RequestFact[] = [
  // Show you how
  {
    id: "wallpaper",
    query: "how do I change my wallpaper",
    whatHappens: "Tells you which app it lives in, then points at each control in turn",
    kind: "teaching",
    group: "show",
    colorToken: "blue",
    targetDemo: "guide",
    sourceApp: "System Settings",
    sampleAnswer: "Open System Settings → Wallpaper. Choose a picture from the grid or click Add Photo.",
  },
  {
    id: "search-box",
    query: "show me where the search box is",
    whatHappens: "Points at it on your screen and waits for you",
    kind: "teaching",
    group: "show",
    colorToken: "teal",
    targetDemo: "guide",
    sourceApp: "System Settings",
    sampleAnswer: "The search field sits at the top of the sidebar. Pointing at it now.",
  },
  {
    id: "reminder",
    query: "how do I add a reminder for tomorrow",
    whatHappens: "Walks you through it in Reminders, and offers to type the text for you",
    kind: "teaching",
    group: "show",
    colorToken: "violet",
    targetDemo: "guide",
    sourceApp: "Reminders",
    sampleAnswer: "Open Reminders. Click + at top right, enter the title, and pick Tomorrow for the date.",
  },

  // Mark things on the page
  {
    id: "mark-start",
    query: "highlight the ones that start with 23",
    whatHappens: "Marks only those numbers, in your colour, right on the page",
    kind: "marking",
    group: "mark",
    colorToken: "green",
    targetDemo: "panel",
    sampleAnswer: 'Marked 10 items starting with "23" in your accent colour on the page. Press esc to clear.',
  },
  {
    id: "mark-word",
    query: 'highlight "Pending"',
    whatHappens: "Marks every place the word appears, and nothing around it",
    kind: "marking",
    group: "mark",
    colorToken: "blue",
    targetDemo: "panel",
    sampleAnswer: 'Marked 4 occurrences of "Pending" on the page. Press esc to clear.',
  },
  {
    id: "mark-end",
    query: "highlight the IDs ending in 07",
    whatHappens: "The same, for the end of the number. Ask to clear the highlighting to take the marks off",
    kind: "marking",
    group: "mark",
    colorToken: "violet",
    targetDemo: "panel",
    sampleAnswer: 'Marked 6 IDs ending in "07" on the page. Press esc to clear.',
  },

  // Read and do
  {
    id: "error",
    query: "what does this error say",
    whatHappens: "Reads what is on screen and explains it in plain words",
    kind: "reading",
    group: "do",
    colorToken: "teal",
    targetDemo: "panel",
    sampleAnswer: "The alert says disk space is running low on Macintosh HD. About 4.2 GB remains free.",
  },
  {
    id: "page",
    query: "what is this page about",
    whatHappens: "Reads the page in front and answers in the panel",
    kind: "reading",
    group: "do",
    colorToken: "blue",
    targetDemo: "panel",
    sampleAnswer: "This page presents VANS: a layer for Mac that points at controls step-by-step or does the task for you.",
  },
  {
    id: "screenshots",
    query: "move the screenshots on my Desktop into a folder called Screenshots",
    whatHappens: "Shows what it will move, waits for you, and can put every file back",
    kind: "files",
    group: "do",
    colorToken: "teal",
    targetDemo: "panel",
  },
];
