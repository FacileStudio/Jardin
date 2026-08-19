export type ArgumentType = "string" | "number" | "boolean";

export interface ArgumentSpec {
  type: ArgumentType;
  required?: boolean;
  enum?: string[];
}

type ArgTS<A extends ArgumentSpec> = A["type"] extends "string"
  ? A extends { enum: readonly (infer E)[] }
    ? E
    : string
  : A["type"] extends "number"
    ? number
    : boolean;

type ArgsOf<Args extends Record<string, ArgumentSpec>> = {
  [K in keyof Args as Args[K]["required"] extends true ? K : never]: ArgTS<Args[K]>;
} & {
  [K in keyof Args as Args[K]["required"] extends true ? never : K]?: ArgTS<Args[K]>;
};

export interface ModelDefinition<Args extends Record<string, ArgumentSpec>> {
  type: string;
  version: string;
  arguments: Args;
  outputs: string[];
  execute: (args: ArgsOf<Args>, env: Record<string, string>) => unknown | Promise<unknown>;
}

// defineModel owns the describe/execute contract internal/flow/model.go expects,
// so a model file only has to declare its schema and its logic. describe prints
// the schema and reads no stdin; execute reads {"arguments","env"} from stdin,
// runs the given function, and prints its return value as the step's stdout. A
// thrown error becomes a stderr message and exit 1, which is what marks the
// step failed to anything depending on it.
export function defineModel<Args extends Record<string, ArgumentSpec>>(
  def: ModelDefinition<Args>,
): void {
  void runModel(def);
}

async function runModel<Args extends Record<string, ArgumentSpec>>(
  def: ModelDefinition<Args>,
): Promise<void> {
  const verb = process.argv[2];

  if (verb === "describe") {
    console.log(
      JSON.stringify({
        type: def.type,
        version: def.version,
        arguments: def.arguments,
        outputs: def.outputs,
      }),
    );
    return;
  }

  if (verb === "execute") {
    const raw = await Bun.stdin.text();
    const input = JSON.parse(raw) as {
      arguments: ArgsOf<Args>;
      env: Record<string, string>;
    };
    try {
      const result = await def.execute(input.arguments, input.env ?? {});
      console.log(JSON.stringify(result));
    } catch (err) {
      console.error(err instanceof Error ? err.message : String(err));
      process.exit(1);
    }
    return;
  }

  console.error(`unknown verb: ${verb}`);
  process.exit(1);
}
