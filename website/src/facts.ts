export const CONTACT_EMAIL = "vedansh@summonvans.in";

export const MAIL_LINKS = {
  tester: "mailto:vedansh@summonvans.in?subject=VANS%20tester&body=Mac%20model%3A%0D%0AmacOS%20version%3A",
  windows: "mailto:vedansh@summonvans.in?subject=VANS%20on%20Windows&body=Windows%20version%3A",
  privacy: "mailto:vedansh@summonvans.in",
} as const;

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
  kind: "teaching" | "reading" | "writing" | "files" | "schedule";
  colorToken: "blue" | "teal" | "green" | "violet";
  targetDemo: "guide" | "panel";
  sampleAnswer?: string;
  sourceApp?: string;
}

export const REQUEST_FACTS: RequestFact[] = [
  {
    id: "wallpaper",
    query: "how do I change my wallpaper",
    whatHappens: "Tells you which app it lives in, then points at each control in turn",
    kind: "teaching",
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
    colorToken: "violet",
    targetDemo: "guide",
    sourceApp: "Reminders",
    sampleAnswer: "Open Reminders. Click + at top right, enter the title, and pick Tomorrow for the date.",
  },
  {
    id: "error",
    query: "what does this error say",
    whatHappens: "Reads what is on screen and explains it in plain words",
    kind: "reading",
    colorToken: "green",
    targetDemo: "panel",
    sampleAnswer: "The alert says disk space is running low on Macintosh HD. About 4.2 GB remains free.",
  },
  {
    id: "page",
    query: "what is this page about",
    whatHappens: "Reads the page in front and answers in the panel",
    kind: "reading",
    colorToken: "blue",
    targetDemo: "panel",
    sampleAnswer: "This page presents VANS: an on-screen guide for Mac that points at controls step-by-step or does the task for you.",
  },
  {
    id: "translate",
    query: 'translate "good morning, see you at three" to Hindi',
    whatHappens: "Works on the quoted or selected text and hands it back to copy",
    kind: "writing",
    colorToken: "violet",
    targetDemo: "panel",
    sampleAnswer: 'सुप्रभात, तीन बजे मिलते हैं (Suprabhat, teen baje milte hain).\n\nCopied to clipboard.',
  },
  {
    id: "screenshots",
    query: "move the screenshots on my Desktop into a folder called Screenshots",
    whatHappens: "Shows what it will move, waits for you, and can put every file back",
    kind: "files",
    colorToken: "teal",
    targetDemo: "panel",
  },
  {
    id: "stretch",
    query: "in 1 minute remind me to stretch",
    whatHappens: "Brings it back at the time. It does not run things behind your back",
    kind: "schedule",
    colorToken: "green",
    targetDemo: "panel",
    sampleAnswer: "Reminder scheduled for 1 minute from now. VANS will post a notification when time is up.",
  },
];
