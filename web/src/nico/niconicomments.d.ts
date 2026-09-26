//#endregion
//#region src/@types/button.d.ts
type ButtonList = {
  left: ButtonPartLeft;
  middle: ButtonPartMiddle[];
  right: ButtonPartRight;
};
type ButtonPartLeft = {
  type: "left";
  left: number;
  top: number;
  width: number;
  height: number;
};
type ButtonPartMiddle = {
  type: "middle";
  left: number;
  top: number;
  width: number;
  height: number;
};
type ButtonPartRight = {
  type: "right";
  right: number;
  top: number;
  height: number;
};
//#endregion
//#region src/@types/canvas.d.ts
type Canvas = HTMLCanvasElement;
type Context2D = CanvasRenderingContext2D;
//#endregion
//#region src/@types/comment.d.ts
type DefaultCommand = {
  color?: string;
  size?: CommentSize;
  font?: CommentFont;
  loc?: CommentLoc;
};
//#endregion
//#region src/contexts/cache.d.ts
declare class ImageCacheContext {
  private _cache;
  get(key: string): {
    image: IRenderer;
    timeout: number;
  } | undefined;
  set(key: string, value: {
    image: IRenderer;
    timeout: number;
  }): void;
  delete(key: string): void;
  reset(): void;
}
//#endregion
//#region src/utils/rangeCache.d.ts
declare class RangeCacheContext {
  readonly reverseActiveOwner: Map<number, boolean>;
  readonly reverseActiveViewer: Map<number, boolean>;
  readonly banActive: Map<number, boolean>;
  reset(): void;
  setCachedActiveState(cache: Map<number, boolean>, vpos: number, result: boolean): void;
}
//#endregion
//#region src/contexts/instanceContext.d.ts
type CommentInstanceContext = {
  config: BaseConfig;
  options: BaseOptions;
  nicoScripts: NicoScript;
  imageCache: ImageCacheContext;
  rangeCache: RangeCacheContext;
};
//#endregion
//#region src/contexts/nicoscript.d.ts
declare const createNicoScripts: () => NicoScript;
declare namespace index_d_exports$4 {
  export { CommentInstanceContext, ImageCacheContext, createNicoScripts };
}
//#endregion
//#region src/comments/BaseComment.d.ts
declare class BaseComment implements IComment {
  protected readonly renderer: IRenderer;
  protected readonly config: BaseConfig;
  protected readonly ctx: CommentInstanceContext;
  protected cacheKey: string;
  comment: FormattedCommentWithSize;
  pos: {
    x: number;
    y: number;
  };
  posY: number;
  readonly pluginName: string;
  image?: IRenderer | null;
  buttonImage?: IRenderer | null;
  index: number;
  private readonly _timeoutIds;
  private _destroyed;
  constructor(comment: FormattedComment, renderer: IRenderer, index: number, ctx: CommentInstanceContext);
  get invisible(): boolean;
  get loc(): "ue" | "naka" | "shita";
  get long(): number;
  get vpos(): number;
  get width(): number;
  get height(): number;
  get flash(): boolean;
  get layer(): number;
  get ignoreScale(): boolean;
  protected getLayerScale(parsedData: FormattedCommentWithFont): number;
  get owner(): boolean;
  get mail(): string[];
  get content(): string;
  set content(_: string);
  protected getCommentSize(parsedData: FormattedCommentWithFont): FormattedCommentWithSize;
  protected parseCommandAndNicoscript(comment: FormattedComment): FormattedCommentWithFont;
  protected parseContent(comment: string): ParseContentResult;
  protected measureText(comment: MeasureTextInput): MeasureTextResult;
  protected convertComment(comment: FormattedComment): FormattedCommentWithSize;
  draw(vpos: number, showCollision: boolean, cursor?: Position, frameActiveState?: FrameActiveState): void;
  protected _draw(posX: number, posY: number, cursor?: Position): void;
  protected _drawRectColor(posX: number, posY: number): void;
  protected _drawBackgroundColor(posX: number, posY: number): void;
  protected _drawDebugInfo(posX: number, posY: number): void;
  protected _drawCollision(posX: number, posY: number, showCollision: boolean): void;
  protected getTextImage(): IRenderer | null;
  protected _generateTextImage(): IRenderer;
  protected _cacheImage(image: IRenderer): void;
  protected canGenerateTextImage(): boolean;
  protected getButtonImage(_posX: number, _posY: number, _cursor?: Position): IRenderer | undefined;
  isHovered(_cursor?: Position, _posX?: number, _posY?: number): boolean;
  protected getCacheKey(): string;
  private _setCommentImageClearTimeout;
  private _setCacheImageExpiryTimeout;
  destroy(): void;
}
//#endregion
//#region src/comments/FlashComment.d.ts
declare class FlashComment extends BaseComment {
  private _globalScale;
  private _buttonImageState?;
  readonly pluginName: string;
  constructor(comment: FormattedComment, renderer: IRenderer, index: number, ctx: CommentInstanceContext);
  private get _flashScriptCharRegex();
  get content(): string;
  get flash(): boolean;
  set content(input: string);
  destroy(): void;
  convertComment(comment: FormattedComment): FormattedCommentWithSize;
  getCommentSize(parsedData: FormattedCommentWithFont): FormattedCommentWithSize;
  parseCommandAndNicoscript(comment: FormattedComment): FormattedCommentWithFont;
  parseContent(input: string, button?: ButtonParams): {
    content: ({
      type: "spacer";
      char: string;
      charWidth: number;
      isButton?: boolean | undefined;
      font?: "defont" | "gulim" | "simsun" | undefined;
      count: number;
    } | {
      type: "text";
      content: string;
      slicedContent: string[];
      isButton?: boolean | undefined;
      font?: "defont" | "gulim" | "simsun" | undefined;
      width?: number[] | undefined;
    })[];
    lineCount: number;
    lineOffset: number;
  };
  measureText(comment: MeasureTextInput): MeasureTextResult;
  private _isDoubleResize;
  private _measureContent;
  _drawCollision(posX: number, posY: number, showCollision: boolean): void;
  _generateTextImage(): IRenderer;
  protected canGenerateTextImage(): boolean;
  getButtonImage(posX: number, posY: number, cursor?: Position): IRenderer | undefined;
  isHovered(_cursor?: Position, _posX?: number, _posY?: number): boolean;
  protected _setupCanvas(renderer: IRenderer): {
    renderer: IRenderer;
  };
}
//#endregion
//#region src/comments/HTML5Comment.d.ts
declare class HTML5Comment extends BaseComment {
  readonly pluginName: string;
  private readonly textImageBoundsCache;
  constructor(comment: FormattedComment, context: IRenderer, index: number, ctx: CommentInstanceContext);
  get content(): string;
  set content(input: string);
  convertComment(comment: FormattedComment): FormattedCommentWithSize;
  getCommentSize(parsedData: FormattedCommentWithFont): FormattedCommentWithSize;
  parseCommandAndNicoscript(comment: FormattedComment): FormattedCommentWithFont;
  parseContent(input: string, font?: HTML5Fonts): {
    content: {
      type: "text";
      content: string;
      slicedContent: string[];
      isButton?: boolean | undefined;
      font?: "defont" | "gulim" | "simsun" | undefined;
      width?: number[] | undefined;
    }[];
    lineCount: number;
    lineOffset: number;
  };
  measureText(comment: MeasureTextInput): MeasureTextResult;
  private _measureComment;
  private _processResizeX;
  _drawCollision(posX: number, posY: number, showCollision: boolean): void;
  protected canGenerateTextImage(): boolean;
  private getTextImageBounds;
  protected _draw(posX: number, posY: number, cursor?: Position): void;
  _generateTextImage(): IRenderer;
  getButtonImage(): undefined;
  isHovered(): boolean;
}
declare namespace index_d_exports$3 {
  export { BaseComment, FlashComment, HTML5Comment };
}
//#endregion
//#region node_modules/.pnpm/valibot@1.4.1_typescript@6.0.3/node_modules/valibot/dist/index.d.mts
//#endregion
//#region src/methods/fallback/fallback.d.ts
/**
 * Fallback type.
 */
type Fallback<TSchema extends BaseSchema<unknown, unknown, BaseIssue<unknown>>> = MaybeDeepReadonly<InferOutput<TSchema>> | ((dataset?: OutputDataset<InferOutput<TSchema>, InferIssue<TSchema>>, config?: Config$1<InferIssue<TSchema>>) => MaybeDeepReadonly<InferOutput<TSchema>>);
/**
 * Schema with fallback type.
 */
type SchemaWithFallback<TSchema extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, TFallback$1 extends Fallback<TSchema>> = TSchema & {
  /**
   * The fallback value.
   */
  readonly fallback: TFallback$1;
};
//#endregion
//#region src/methods/fallback/fallbackAsync.d.ts
/**
 * Fallback async type.
 */
type FallbackAsync<TSchema extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>> = MaybeDeepReadonly<InferOutput<TSchema>> | ((dataset?: OutputDataset<InferOutput<TSchema>, InferIssue<TSchema>>, config?: Config$1<InferIssue<TSchema>>) => MaybePromise<MaybeDeepReadonly<InferOutput<TSchema>>>);
/**
 * Schema with fallback async type.
 */
type SchemaWithFallbackAsync<TSchema extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TFallback$1 extends FallbackAsync<TSchema>> = Omit<TSchema, "async" | "~standard" | "~run"> & {
  /**
   * The fallback value.
   */
  readonly fallback: TFallback$1;
  /**
   * Whether it's async.
   */
  readonly async: true;
  /**
   * The Standard Schema properties.
   *
   * @internal
   */
  readonly "~standard": StandardProps<InferInput<TSchema>, InferOutput<TSchema>>;
  /**
   * Parses unknown input values.
   *
   * @param dataset The input dataset.
   * @param config The configuration.
   *
   * @returns The output dataset.
   *
   * @internal
   */
  readonly "~run": (dataset: UnknownDataset, config: Config$1<BaseIssue<unknown>>) => Promise<OutputDataset<InferOutput<TSchema>, InferIssue<TSchema>>>;
};
//#endregion
//#region src/methods/pipe/pipe.d.ts
/**
 * Schema with pipe type.
 */
type SchemaWithPipe<TPipe$1 extends readonly [BaseSchema<unknown, unknown, BaseIssue<unknown>>, ...PipeItem<any, unknown, BaseIssue<unknown>>[]]> = Omit<FirstTupleItem<TPipe$1>, "pipe" | "~standard" | "~run" | "~types"> & {
  /**
   * The pipe items.
   */
  readonly pipe: TPipe$1;
  /**
   * The Standard Schema properties.
   *
   * @internal
   */
  readonly "~standard": StandardProps<InferInput<FirstTupleItem<TPipe$1>>, InferOutput<LastTupleItem<TPipe$1>>>;
  /**
   * Parses unknown input values.
   *
   * @param dataset The input dataset.
   * @param config The configuration.
   *
   * @returns The output dataset.
   *
   * @internal
   */
  readonly "~run": (dataset: UnknownDataset, config: Config$1<BaseIssue<unknown>>) => OutputDataset<InferOutput<LastTupleItem<TPipe$1>>, InferIssue<TPipe$1[number]>>;
  /**
   * The input, output and issue type.
   *
   * @internal
   */
  readonly "~types"?: {
    readonly input: InferInput<FirstTupleItem<TPipe$1>>;
    readonly output: InferOutput<LastTupleItem<TPipe$1>>;
    readonly issue: InferIssue<TPipe$1[number]>;
  } | undefined;
};
//#endregion
//#region src/types/metadata.d.ts
/**
 * Base metadata interface.
 */
interface BaseMetadata<TInput$1> {
  /**
   * The object kind.
   */
  readonly kind: "metadata";
  /**
   * The metadata type.
   */
  readonly type: string;
  /**
   * The metadata reference.
   */
  readonly reference: (...args: any[]) => BaseMetadata<any>;
  /**
   * The input, output and issue type.
   *
   * @internal
   */
  readonly "~types"?: {
    readonly input: TInput$1;
    readonly output: TInput$1;
    readonly issue: never;
  } | undefined;
}
//#endregion
//#region src/types/dataset.d.ts
/**
 * Unknown dataset interface.
 */
interface UnknownDataset {
  /**
   * Whether is's typed.
   */
  typed?: false;
  /**
   * The dataset value.
   */
  value: unknown;
  /**
   * The dataset issues.
   */
  issues?: undefined;
}
/**
 * Success dataset interface.
 */
interface SuccessDataset<TValue$1> {
  /**
   * Whether is's typed.
   */
  typed: true;
  /**
   * The dataset value.
   */
  value: TValue$1;
  /**
   * The dataset issues.
   */
  issues?: undefined;
}
/**
 * Partial dataset interface.
 */
interface PartialDataset<TValue$1, TIssue extends BaseIssue<unknown>> {
  /**
   * Whether is's typed.
   */
  typed: true;
  /**
   * The dataset value.
   */
  value: TValue$1;
  /**
   * The dataset issues.
   */
  issues: [TIssue, ...TIssue[]];
}
/**
 * Failure dataset interface.
 */
interface FailureDataset<TIssue extends BaseIssue<unknown>> {
  /**
   * Whether is's typed.
   */
  typed: false;
  /**
   * The dataset value.
   */
  value: unknown;
  /**
   * The dataset issues.
   */
  issues: [TIssue, ...TIssue[]];
}
/**
 * Output dataset type.
 */
type OutputDataset<TValue$1, TIssue extends BaseIssue<unknown>> = SuccessDataset<TValue$1> | PartialDataset<TValue$1, TIssue> | FailureDataset<TIssue>;
//#endregion
//#region src/types/standard.d.ts
/**
 * The Standard Schema properties interface.
 */
interface StandardProps<TInput$1, TOutput$1> {
  /**
   * The version number of the standard.
   */
  readonly version: 1;
  /**
   * The vendor name of the schema library.
   */
  readonly vendor: "valibot";
  /**
   * Validates unknown input values.
   */
  readonly validate: (value: unknown) => StandardResult<TOutput$1> | Promise<StandardResult<TOutput$1>>;
  /**
   * Inferred types associated with the schema.
   */
  readonly types?: StandardTypes<TInput$1, TOutput$1> | undefined;
}
/**
 * The result interface of the validate function.
 */
type StandardResult<TOutput$1> = StandardSuccessResult<TOutput$1> | StandardFailureResult;
/**
 * The result interface if validation succeeds.
 */
interface StandardSuccessResult<TOutput$1> {
  /**
   * The typed output value.
   */
  readonly value: TOutput$1;
  /**
   * The non-existent issues.
   */
  readonly issues?: undefined;
}
/**
 * The result interface if validation fails.
 */
interface StandardFailureResult {
  /**
   * The issues of failed validation.
   */
  readonly issues: readonly StandardIssue[];
}
/**
 * The issue interface of the failure output.
 */
interface StandardIssue {
  /**
   * The error message of the issue.
   */
  readonly message: string;
  /**
   * The path of the issue, if any.
   */
  readonly path?: readonly (PropertyKey | StandardPathItem)[] | undefined;
}
/**
 * The path item interface of the issue.
 */
interface StandardPathItem {
  /**
   * The key of the path item.
   */
  readonly key: PropertyKey;
}
/**
 * The Standard Schema types interface.
 */
interface StandardTypes<TInput$1, TOutput$1> {
  /**
   * The input type of the schema.
   */
  readonly input: TInput$1;
  /**
   * The output type of the schema.
   */
  readonly output: TOutput$1;
}
//#endregion
//#region src/types/schema.d.ts
/**
 * Base schema interface.
 */
interface BaseSchema<TInput$1, TOutput$1, TIssue extends BaseIssue<unknown>> {
  /**
   * The object kind.
   */
  readonly kind: "schema";
  /**
   * The schema type.
   */
  readonly type: string;
  /**
   * The schema reference.
   */
  readonly reference: (...args: any[]) => BaseSchema<unknown, unknown, BaseIssue<unknown>>;
  /**
   * The expected property.
   */
  readonly expects: string;
  /**
   * Whether it's async.
   */
  readonly async: false;
  /**
   * The Standard Schema properties.
   *
   * @internal
   */
  readonly "~standard": StandardProps<TInput$1, TOutput$1>;
  /**
   * Parses unknown input values.
   *
   * @param dataset The input dataset.
   * @param config The configuration.
   *
   * @returns The output dataset.
   *
   * @internal
   */
  readonly "~run": (dataset: UnknownDataset, config: Config$1<BaseIssue<unknown>>) => OutputDataset<TOutput$1, TIssue>;
  /**
   * The input, output and issue type.
   *
   * @internal
   */
  readonly "~types"?: {
    readonly input: TInput$1;
    readonly output: TOutput$1;
    readonly issue: TIssue;
  } | undefined;
}
/**
 * Base schema async interface.
 */
interface BaseSchemaAsync<TInput$1, TOutput$1, TIssue extends BaseIssue<unknown>> extends Omit<BaseSchema<TInput$1, TOutput$1, TIssue>, "reference" | "async" | "~run"> {
  /**
   * The schema reference.
   */
  readonly reference: (...args: any[]) => BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>;
  /**
   * Whether it's async.
   */
  readonly async: true;
  /**
   * Parses unknown input values.
   *
   * @param dataset The input dataset.
   * @param config The configuration.
   *
   * @returns The output dataset.
   *
   * @internal
   */
  readonly "~run": (dataset: UnknownDataset, config: Config$1<BaseIssue<unknown>>) => Promise<OutputDataset<TOutput$1, TIssue>>;
}
//#endregion
//#region src/types/transformation.d.ts
/**
 * Base transformation interface.
 */
interface BaseTransformation<TInput$1, TOutput$1, TIssue extends BaseIssue<unknown>> {
  /**
   * The object kind.
   */
  readonly kind: "transformation";
  /**
   * The transformation type.
   */
  readonly type: string;
  /**
   * The transformation reference.
   */
  readonly reference: (...args: any[]) => BaseTransformation<any, any, BaseIssue<unknown>>;
  /**
   * Whether it's async.
   */
  readonly async: false;
  /**
   * Transforms known input values.
   *
   * @param dataset The input dataset.
   * @param config The configuration.
   *
   * @returns The output dataset.
   *
   * @internal
   */
  readonly "~run": (dataset: SuccessDataset<TInput$1>, config: Config$1<BaseIssue<unknown>>) => OutputDataset<TOutput$1, BaseIssue<unknown> | TIssue>;
  /**
   * The input, output and issue type.
   *
   * @internal
   */
  readonly "~types"?: {
    readonly input: TInput$1;
    readonly output: TOutput$1;
    readonly issue: TIssue;
  } | undefined;
}
/**
 * Base transformation async interface.
 */
interface BaseTransformationAsync<TInput$1, TOutput$1, TIssue extends BaseIssue<unknown>> extends Omit<BaseTransformation<TInput$1, TOutput$1, TIssue>, "reference" | "async" | "~run"> {
  /**
   * The transformation reference.
   */
  readonly reference: (...args: any[]) => BaseTransformation<any, any, BaseIssue<unknown>> | BaseTransformationAsync<any, any, BaseIssue<unknown>>;
  /**
   * Whether it's async.
   */
  readonly async: true;
  /**
   * Transforms known input values.
   *
   * @param dataset The input dataset.
   * @param config The configuration.
   *
   * @returns The output dataset.
   *
   * @internal
   */
  readonly "~run": (dataset: SuccessDataset<TInput$1>, config: Config$1<BaseIssue<unknown>>) => Promise<OutputDataset<TOutput$1, BaseIssue<unknown> | TIssue>>;
}
//#endregion
//#region src/types/validation.d.ts
/**
 * Base validation interface.
 */
interface BaseValidation<TInput$1, TOutput$1, TIssue extends BaseIssue<unknown>> {
  /**
   * The object kind.
   */
  readonly kind: "validation";
  /**
   * The validation type.
   */
  readonly type: string;
  /**
   * The validation reference.
   */
  readonly reference: (...args: any[]) => BaseValidation<any, any, BaseIssue<unknown>>;
  /**
   * The expected property.
   */
  readonly expects: string | null;
  /**
   * Whether it's async.
   */
  readonly async: false;
  /**
   * Validates known input values.
   *
   * @param dataset The input dataset.
   * @param config The configuration.
   *
   * @returns The output dataset.
   *
   * @internal
   */
  readonly "~run": (dataset: OutputDataset<TInput$1, BaseIssue<unknown>>, config: Config$1<BaseIssue<unknown>>) => OutputDataset<TOutput$1, BaseIssue<unknown> | TIssue>;
  /**
   * The input, output and issue type.
   *
   * @internal
   */
  readonly "~types"?: {
    readonly input: TInput$1;
    readonly output: TOutput$1;
    readonly issue: TIssue;
  } | undefined;
}
/**
 * Base validation async interface.
 */
interface BaseValidationAsync<TInput$1, TOutput$1, TIssue extends BaseIssue<unknown>> extends Omit<BaseValidation<TInput$1, TOutput$1, TIssue>, "reference" | "async" | "~run"> {
  /**
   * The validation reference.
   */
  readonly reference: (...args: any[]) => BaseValidation<any, any, BaseIssue<unknown>> | BaseValidationAsync<any, any, BaseIssue<unknown>>;
  /**
   * Whether it's async.
   */
  readonly async: true;
  /**
   * Validates known input values.
   *
   * @param dataset The input dataset.
   * @param config The configuration.
   *
   * @returns The output dataset.
   *
   * @internal
   */
  readonly "~run": (dataset: OutputDataset<TInput$1, BaseIssue<unknown>>, config: Config$1<BaseIssue<unknown>>) => Promise<OutputDataset<TOutput$1, BaseIssue<unknown> | TIssue>>;
}
//#endregion
//#region src/types/infer.d.ts
/**
 * Infer input type.
 */
type InferInput<TItem$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>> | BaseValidation<any, unknown, BaseIssue<unknown>> | BaseValidationAsync<any, unknown, BaseIssue<unknown>> | BaseTransformation<any, unknown, BaseIssue<unknown>> | BaseTransformationAsync<any, unknown, BaseIssue<unknown>> | BaseMetadata<any>> = NonNullable<TItem$1["~types"]>["input"];
/**
 * Infer output type.
 */
type InferOutput<TItem$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>> | BaseValidation<any, unknown, BaseIssue<unknown>> | BaseValidationAsync<any, unknown, BaseIssue<unknown>> | BaseTransformation<any, unknown, BaseIssue<unknown>> | BaseTransformationAsync<any, unknown, BaseIssue<unknown>> | BaseMetadata<any>> = NonNullable<TItem$1["~types"]>["output"];
/**
 * Infer issue type.
 */
type InferIssue<TItem$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>> | BaseValidation<any, unknown, BaseIssue<unknown>> | BaseValidationAsync<any, unknown, BaseIssue<unknown>> | BaseTransformation<any, unknown, BaseIssue<unknown>> | BaseTransformationAsync<any, unknown, BaseIssue<unknown>> | BaseMetadata<any>> = NonNullable<TItem$1["~types"]>["issue"];
/**
 * Checks if a type is `never`.
 */
type IsNever<Type> = [Type] extends [never] ? true : false;
/**
 * Constructs a type that is maybe readonly.
 */
type MaybeReadonly<TValue$1> = TValue$1 | Readonly<TValue$1>;
/**
 * Constructs a type that is deeply readonly.
 */
type DeepReadonly<TValue$1> = TValue$1 extends Record<string, unknown> | readonly unknown[] ? { readonly [TKey in keyof TValue$1]: DeepReadonly<TValue$1[TKey]>; } : TValue$1;
/**
 * Constructs a type that is maybe deeply readonly.
 */
type MaybeDeepReadonly<TValue$1> = TValue$1 | DeepReadonly<TValue$1>;
/**
 * Constructs a type that is maybe a promise.
 */
type MaybePromise<TValue$1> = TValue$1 | Promise<TValue$1>;
/**
 * Prettifies a type for better readability.
 *
 * Hint: This type has no effect and is only used so that TypeScript displays
 * the final type in the preview instead of the utility types used.
 */
type Prettify<TObject> = { [TKey in keyof TObject]: TObject[TKey]; } & {};
/**
 * Marks specific keys as optional.
 */
type MarkOptional<TObject, TKeys extends keyof TObject> = { [TKey in keyof TObject]?: unknown; } & Omit<TObject, TKeys> & Partial<Pick<TObject, TKeys>>;
/**
 * Extracts first tuple item.
 */
type FirstTupleItem<TTuple extends readonly [unknown, ...unknown[]]> = TTuple[0];
/**
 * Extracts last tuple item.
 */
type LastTupleItem<TTuple extends readonly [unknown, ...unknown[]]> = TTuple[TTuple extends readonly [unknown, ...infer TRest] ? TRest["length"] : never];
/**
 * Converts union to intersection type.
 */
type UnionToIntersect<TUnion> = (TUnion extends any ? (arg: TUnion) => void : never) extends ((arg: infer Intersect) => void) ? Intersect : never;
//#endregion
//#region src/types/other.d.ts
/**
 * Error message type.
 */
type ErrorMessage<TIssue extends BaseIssue<unknown>> = ((issue: TIssue) => string) | string;
/**
 * Default type.
 */
type Default<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, TInput$1 extends null | undefined> = MaybeDeepReadonly<InferInput<TWrapped$1> | TInput$1> | ((dataset?: UnknownDataset, config?: Config$1<InferIssue<TWrapped$1>>) => MaybeDeepReadonly<InferInput<TWrapped$1> | TInput$1>) | undefined;
/**
 * Default async type.
 */
type DefaultAsync<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TInput$1 extends null | undefined> = MaybeDeepReadonly<InferInput<TWrapped$1> | TInput$1> | ((dataset?: UnknownDataset, config?: Config$1<InferIssue<TWrapped$1>>) => MaybePromise<MaybeDeepReadonly<InferInput<TWrapped$1> | TInput$1>>) | undefined;
/**
 * Default value type.
 */
type DefaultValue<TDefault extends Default<BaseSchema<unknown, unknown, BaseIssue<unknown>>, null | undefined> | DefaultAsync<BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, null | undefined>> = TDefault extends DefaultAsync<infer TWrapped extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, infer TInput> ? TDefault extends ((dataset?: UnknownDataset, config?: Config$1<InferIssue<TWrapped>>) => MaybePromise<MaybeDeepReadonly<InferInput<TWrapped> | TInput>>) ? Awaited<ReturnType<TDefault>> : TDefault : never;
//#endregion
//#region src/types/object.d.ts
/**
 * Optional entry schema type.
 */
type OptionalEntrySchema = ExactOptionalSchema<BaseSchema<unknown, unknown, BaseIssue<unknown>>, unknown> | NullishSchema<BaseSchema<unknown, unknown, BaseIssue<unknown>>, unknown> | OptionalSchema<BaseSchema<unknown, unknown, BaseIssue<unknown>>, unknown>;
/**
 * Optional entry schema async type.
 */
type OptionalEntrySchemaAsync = ExactOptionalSchemaAsync<BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, unknown> | NullishSchemaAsync<BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, unknown> | OptionalSchemaAsync<BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, unknown>;
/**
 * Object entries interface.
 */
interface ObjectEntries {
  [key: string]: BaseSchema<unknown, unknown, BaseIssue<unknown>> | SchemaWithFallback<BaseSchema<unknown, unknown, BaseIssue<unknown>>, unknown> | OptionalEntrySchema;
}
/**
 * Object entries async interface.
 */
interface ObjectEntriesAsync {
  [key: string]: BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>> | SchemaWithFallback<BaseSchema<unknown, unknown, BaseIssue<unknown>>, unknown> | SchemaWithFallbackAsync<BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, unknown> | OptionalEntrySchema | OptionalEntrySchemaAsync;
}
/**
 * Infer entries input type.
 */
type InferEntriesInput<TEntries$1 extends ObjectEntries | ObjectEntriesAsync> = { -readonly [TKey in keyof TEntries$1]: InferInput<TEntries$1[TKey]>; };
/**
 * Infer entries output type.
 */
type InferEntriesOutput<TEntries$1 extends ObjectEntries | ObjectEntriesAsync> = { -readonly [TKey in keyof TEntries$1]: InferOutput<TEntries$1[TKey]>; };
/**
 * Optional input keys type.
 */
type OptionalInputKeys<TEntries$1 extends ObjectEntries | ObjectEntriesAsync> = { [TKey in keyof TEntries$1]: TEntries$1[TKey] extends OptionalEntrySchema | OptionalEntrySchemaAsync ? TKey : never; }[keyof TEntries$1];
/**
 * Optional output keys type.
 */
type OptionalOutputKeys<TEntries$1 extends ObjectEntries | ObjectEntriesAsync> = { [TKey in keyof TEntries$1]: TEntries$1[TKey] extends OptionalEntrySchema | OptionalEntrySchemaAsync ? undefined extends TEntries$1[TKey]["default"] ? TKey : never : never; }[keyof TEntries$1];
/**
 * Input with question marks type.
 */
type InputWithQuestionMarks<TEntries$1 extends ObjectEntries | ObjectEntriesAsync, TObject extends InferEntriesInput<TEntries$1>> = MarkOptional<TObject, OptionalInputKeys<TEntries$1>>;
/**
 * Output with question marks type.
 */
type OutputWithQuestionMarks<TEntries$1 extends ObjectEntries | ObjectEntriesAsync, TObject extends InferEntriesOutput<TEntries$1>> = MarkOptional<TObject, OptionalOutputKeys<TEntries$1>>;
/**
 * Readonly output keys type.
 */
type ReadonlyOutputKeys<TEntries$1 extends ObjectEntries | ObjectEntriesAsync> = { [TKey in keyof TEntries$1]: TEntries$1[TKey] extends {
  readonly pipe: readonly unknown[];
} ? ReadonlyAction<any> extends TEntries$1[TKey]["pipe"][number] ? TKey : never : never; }[keyof TEntries$1];
/**
 * Output with readonly type.
 */
type OutputWithReadonly<TEntries$1 extends ObjectEntries | ObjectEntriesAsync, TObject extends OutputWithQuestionMarks<TEntries$1, InferEntriesOutput<TEntries$1>>> = ReadonlyOutputKeys<TEntries$1> extends never ? TObject : Readonly<TObject> & Pick<TObject, Exclude<keyof TObject, ReadonlyOutputKeys<TEntries$1>>>;
/**
 * Infer object input type.
 */
type InferObjectInput<TEntries$1 extends ObjectEntries | ObjectEntriesAsync> = Prettify<InputWithQuestionMarks<TEntries$1, InferEntriesInput<TEntries$1>>>;
/**
 * Infer object output type.
 */
type InferObjectOutput<TEntries$1 extends ObjectEntries | ObjectEntriesAsync> = Prettify<OutputWithReadonly<TEntries$1, OutputWithQuestionMarks<TEntries$1, InferEntriesOutput<TEntries$1>>>>;
/**
 * Infer object issue type.
 */
type InferObjectIssue<TEntries$1 extends ObjectEntries | ObjectEntriesAsync> = InferIssue<TEntries$1[keyof TEntries$1]>;
//#endregion
//#region src/types/issue.d.ts
/**
 * Array path item interface.
 */
interface ArrayPathItem {
  /**
   * The path item type.
   */
  readonly type: "array";
  /**
   * The path item origin.
   */
  readonly origin: "value";
  /**
   * The path item input.
   */
  readonly input: MaybeReadonly<unknown[]>;
  /**
   * The path item key.
   */
  readonly key: number;
  /**
   * The path item value.
   */
  readonly value: unknown;
}
/**
 * Map path item interface.
 */
interface MapPathItem {
  /**
   * The path item type.
   */
  readonly type: "map";
  /**
   * The path item origin.
   */
  readonly origin: "key" | "value";
  /**
   * The path item input.
   */
  readonly input: Map<unknown, unknown>;
  /**
   * The path item key.
   */
  readonly key: unknown;
  /**
   * The path item value.
   */
  readonly value: unknown;
}
/**
 * Object path item interface.
 */
interface ObjectPathItem {
  /**
   * The path item type.
   */
  readonly type: "object";
  /**
   * The path item origin.
   */
  readonly origin: "key" | "value";
  /**
   * The path item input.
   */
  readonly input: Record<string, unknown>;
  /**
   * The path item key.
   */
  readonly key: string;
  /**
   * The path item value.
   */
  readonly value: unknown;
}
/**
 * Set path item interface.
 */
interface SetPathItem {
  /**
   * The path item type.
   */
  readonly type: "set";
  /**
   * The path item origin.
   */
  readonly origin: "value";
  /**
   * The path item input.
   */
  readonly input: Set<unknown>;
  /**
   * The path item key.
   */
  readonly key: null;
  /**
   * The path item key.
   */
  readonly value: unknown;
}
/**
 * Unknown path item interface.
 */
interface UnknownPathItem {
  /**
   * The path item type.
   */
  readonly type: "unknown";
  /**
   * The path item origin.
   */
  readonly origin: "key" | "value";
  /**
   * The path item input.
   */
  readonly input: unknown;
  /**
   * The path item key.
   */
  readonly key: unknown;
  /**
   * The path item value.
   */
  readonly value: unknown;
}
/**
 * Issue path item type.
 */
type IssuePathItem = ArrayPathItem | MapPathItem | ObjectPathItem | SetPathItem | UnknownPathItem;
/**
 * Base issue interface.
 */
interface BaseIssue<TInput$1> extends Config$1<BaseIssue<TInput$1>> {
  /**
   * The issue kind.
   */
  readonly kind: "schema" | "validation" | "transformation";
  /**
   * The issue type.
   */
  readonly type: string;
  /**
   * The raw input data.
   */
  readonly input: TInput$1;
  /**
   * The expected property.
   */
  readonly expected: string | null;
  /**
   * The received property.
   */
  readonly received: string;
  /**
   * The error message.
   */
  readonly message: string;
  /**
   * The input requirement.
   */
  readonly requirement?: unknown | undefined;
  /**
   * The issue path.
   */
  readonly path?: [IssuePathItem, ...IssuePathItem[]] | undefined;
  /**
   * The sub issues.
   */
  readonly issues?: [BaseIssue<TInput$1>, ...BaseIssue<TInput$1>[]] | undefined;
}
//#endregion
//#region src/types/config.d.ts
/**
 * Config interface.
 */
interface Config$1<TIssue extends BaseIssue<unknown>> {
  /**
   * The selected language.
   */
  readonly lang?: string | undefined;
  /**
   * The error message.
   */
  readonly message?: ErrorMessage<TIssue> | undefined;
  /**
   * Whether it should be aborted early.
   */
  readonly abortEarly?: boolean | undefined;
  /**
   * Whether a pipe should be aborted early.
   */
  readonly abortPipeEarly?: boolean | undefined;
}
//#endregion
//#region src/types/pipe.d.ts
/**
 * Pipe action type.
 */
type PipeAction<TInput$1, TOutput$1, TIssue extends BaseIssue<unknown>> = BaseValidation<TInput$1, TOutput$1, TIssue> | BaseTransformation<TInput$1, TOutput$1, TIssue> | BaseMetadata<TInput$1>;
/**
 * Pipe item type.
 */
type PipeItem<TInput$1, TOutput$1, TIssue extends BaseIssue<unknown>> = BaseSchema<TInput$1, TOutput$1, TIssue> | PipeAction<TInput$1, TOutput$1, TIssue>;
//#endregion
//#region src/schemas/array/types.d.ts
/**
 * Array issue interface.
 */
interface ArrayIssue extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "array";
  /**
   * The expected property.
   */
  readonly expected: "Array";
}
//#endregion
//#region src/schemas/array/array.d.ts
/**
 * Array schema interface.
 */
interface ArraySchema<TItem$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, TMessage extends ErrorMessage<ArrayIssue> | undefined> extends BaseSchema<InferInput<TItem$1>[], InferOutput<TItem$1>[], ArrayIssue | InferIssue<TItem$1>> {
  /**
   * The schema type.
   */
  readonly type: "array";
  /**
   * The schema reference.
   */
  readonly reference: typeof array;
  /**
   * The expected property.
   */
  readonly expects: "Array";
  /**
   * The array item schema.
   */
  readonly item: TItem$1;
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates an array schema.
 *
 * @param item The item schema.
 *
 * @returns An array schema.
 */
declare function array<const TItem$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>>(item: TItem$1): ArraySchema<TItem$1, undefined>;
/**
 * Creates an array schema.
 *
 * @param item The item schema.
 * @param message The error message.
 *
 * @returns An array schema.
 */
declare function array<const TItem$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, const TMessage extends ErrorMessage<ArrayIssue> | undefined>(item: TItem$1, message: TMessage): ArraySchema<TItem$1, TMessage>;
//#endregion
//#region src/schemas/boolean/boolean.d.ts
/**
 * Boolean issue interface.
 */
interface BooleanIssue extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "boolean";
  /**
   * The expected property.
   */
  readonly expected: "boolean";
}
/**
 * Boolean schema interface.
 */
interface BooleanSchema<TMessage extends ErrorMessage<BooleanIssue> | undefined> extends BaseSchema<boolean, boolean, BooleanIssue> {
  /**
   * The schema type.
   */
  readonly type: "boolean";
  /**
   * The schema reference.
   */
  readonly reference: typeof boolean;
  /**
   * The expected property.
   */
  readonly expects: "boolean";
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates a boolean schema.
 *
 * @returns A boolean schema.
 */
declare function boolean(): BooleanSchema<undefined>;
/**
 * Creates a boolean schema.
 *
 * @param message The error message.
 *
 * @returns A boolean schema.
 */
declare function boolean<const TMessage extends ErrorMessage<BooleanIssue> | undefined>(message: TMessage): BooleanSchema<TMessage>;
//#endregion
//#region src/schemas/exactOptional/exactOptional.d.ts
/**
 * Exact optional schema interface.
 */
interface ExactOptionalSchema<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, TDefault extends Default<TWrapped$1, never>> extends BaseSchema<InferInput<TWrapped$1>, InferOutput<TWrapped$1>, InferIssue<TWrapped$1>> {
  /**
   * The schema type.
   */
  readonly type: "exact_optional";
  /**
   * The schema reference.
   */
  readonly reference: typeof exactOptional;
  /**
   * The expected property.
   */
  readonly expects: TWrapped$1["expects"];
  /**
   * The wrapped schema.
   */
  readonly wrapped: TWrapped$1;
  /**
   * The default value.
   */
  readonly default: TDefault;
}
/**
 * Creates an exact optional schema.
 *
 * @param wrapped The wrapped schema.
 *
 * @returns An exact optional schema.
 */
declare function exactOptional<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>>(wrapped: TWrapped$1): ExactOptionalSchema<TWrapped$1, undefined>;
/**
 * Creates an exact optional schema.
 *
 * @param wrapped The wrapped schema.
 * @param default_ The default value.
 *
 * @returns An exact optional schema.
 */
declare function exactOptional<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, const TDefault extends Default<TWrapped$1, never>>(wrapped: TWrapped$1, default_: TDefault): ExactOptionalSchema<TWrapped$1, TDefault>;
//#endregion
//#region src/schemas/exactOptional/exactOptionalAsync.d.ts
/**
 * Exact optional schema async interface.
 */
interface ExactOptionalSchemaAsync<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TDefault extends DefaultAsync<TWrapped$1, never>> extends BaseSchemaAsync<InferInput<TWrapped$1>, InferOutput<TWrapped$1>, InferIssue<TWrapped$1>> {
  /**
   * The schema type.
   */
  readonly type: "exact_optional";
  /**
   * The schema reference.
   */
  readonly reference: typeof exactOptional | typeof exactOptionalAsync;
  /**
   * The expected property.
   */
  readonly expects: TWrapped$1["expects"];
  /**
   * The wrapped schema.
   */
  readonly wrapped: TWrapped$1;
  /**
   * The default value.
   */
  readonly default: TDefault;
}
/**
 * Creates an exact optional schema.
 *
 * @param wrapped The wrapped schema.
 *
 * @returns An exact optional schema.
 */
declare function exactOptionalAsync<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>>(wrapped: TWrapped$1): ExactOptionalSchemaAsync<TWrapped$1, undefined>;
/**
 * Creates an exact optional schema.
 *
 * @param wrapped The wrapped schema.
 * @param default_ The default value.
 *
 * @returns An exact optional schema.
 */
declare function exactOptionalAsync<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, const TDefault extends DefaultAsync<TWrapped$1, never>>(wrapped: TWrapped$1, default_: TDefault): ExactOptionalSchemaAsync<TWrapped$1, TDefault>;
//#endregion
//#region src/schemas/intersect/types.d.ts
/**
 * Intersect issue interface.
 */
interface IntersectIssue extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "intersect";
  /**
   * The expected property.
   */
  readonly expected: string;
}
/**
 * Intersect options type.
 */
type IntersectOptions = MaybeReadonly<BaseSchema<unknown, unknown, BaseIssue<unknown>>[]>;
/**
 * Intersect options async type.
 */
type IntersectOptionsAsync = MaybeReadonly<(BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>)[]>;
/**
 * Infer option type.
 */
type InferOption<TInput$1, TOutput$1> = BaseSchema<TInput$1, TOutput$1, BaseIssue<unknown>> | BaseSchemaAsync<TInput$1, TOutput$1, BaseIssue<unknown>>;
/**
 * Infer intersect input type.
 */
type InferIntersectInput<TOptions$1 extends IntersectOptions | IntersectOptionsAsync> = TOptions$1 extends readonly [InferOption<infer TInput, unknown>, ...infer TRest] ? TRest extends readonly [InferOption<unknown, unknown>, ...InferOption<unknown, unknown>[]] ? TInput & InferIntersectInput<TRest> : TInput : IsNever<TOptions$1[number]> extends true ? never : UnionToIntersect<InferInput<TOptions$1[number]>>;
/**
 * Infer intersect output type.
 */
type InferIntersectOutput<TOptions$1 extends IntersectOptions | IntersectOptionsAsync> = TOptions$1 extends readonly [InferOption<unknown, infer TOutput>, ...infer TRest] ? TRest extends readonly [InferOption<unknown, unknown>, ...InferOption<unknown, unknown>[]] ? TOutput & InferIntersectOutput<TRest> : TOutput : IsNever<TOptions$1[number]> extends true ? never : UnionToIntersect<InferOutput<TOptions$1[number]>>;
//#endregion
//#region src/schemas/intersect/intersect.d.ts
/**
 * Intersect schema interface.
 */
interface IntersectSchema<TOptions$1 extends IntersectOptions, TMessage extends ErrorMessage<IntersectIssue> | undefined> extends BaseSchema<InferIntersectInput<TOptions$1>, InferIntersectOutput<TOptions$1>, IntersectIssue | InferIssue<TOptions$1[number]>> {
  /**
   * The schema type.
   */
  readonly type: "intersect";
  /**
   * The schema reference.
   */
  readonly reference: typeof intersect;
  /**
   * The intersect options.
   */
  readonly options: TOptions$1;
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates an intersect schema.
 *
 * @param options The intersect options.
 *
 * @returns An intersect schema.
 */
declare function intersect<const TOptions$1 extends IntersectOptions>(options: TOptions$1): IntersectSchema<TOptions$1, undefined>;
/**
 * Creates an intersect schema.
 *
 * @param options The intersect options.
 * @param message The error message.
 *
 * @returns An intersect schema.
 */
declare function intersect<const TOptions$1 extends IntersectOptions, const TMessage extends ErrorMessage<IntersectIssue> | undefined>(options: TOptions$1, message: TMessage): IntersectSchema<TOptions$1, TMessage>;
//#endregion
//#region src/schemas/literal/literal.d.ts
/**
 * Literal type.
 */
type Literal = bigint | boolean | number | string | symbol;
/**
 * Literal issue interface.
 */
interface LiteralIssue extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "literal";
  /**
   * The expected property.
   */
  readonly expected: string;
}
/**
 * Literal schema interface.
 */
interface LiteralSchema<TLiteral extends Literal, TMessage extends ErrorMessage<LiteralIssue> | undefined> extends BaseSchema<TLiteral, TLiteral, LiteralIssue> {
  /**
   * The schema type.
   */
  readonly type: "literal";
  /**
   * The schema reference.
   */
  readonly reference: typeof literal;
  /**
   * The literal value.
   */
  readonly literal: TLiteral;
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates a literal schema.
 *
 * @param literal_ The literal value.
 *
 * @returns A literal schema.
 */
declare function literal<const TLiteral extends Literal>(literal_: TLiteral): LiteralSchema<TLiteral, undefined>;
/**
 * Creates a literal schema.
 *
 * @param literal_ The literal value.
 * @param message The error message.
 *
 * @returns A literal schema.
 */
declare function literal<const TLiteral extends Literal, const TMessage extends ErrorMessage<LiteralIssue> | undefined>(literal_: TLiteral, message: TMessage): LiteralSchema<TLiteral, TMessage>;
//#endregion
//#region src/schemas/union/types.d.ts
/**
 * Union issue interface.
 */
interface UnionIssue<TSubIssue extends BaseIssue<unknown>> extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "union";
  /**
   * The expected property.
   */
  readonly expected: string;
  /**
   * The sub issues.
   */
  readonly issues?: [TSubIssue, ...TSubIssue[]];
}
//#endregion
//#region src/schemas/union/union.d.ts
/**
 * Union options type.
 */
type UnionOptions = MaybeReadonly<BaseSchema<unknown, unknown, BaseIssue<unknown>>[]>;
/**
 * Union schema interface.
 */
interface UnionSchema<TOptions$1 extends UnionOptions, TMessage extends ErrorMessage<UnionIssue<InferIssue<TOptions$1[number]>>> | undefined> extends BaseSchema<InferInput<TOptions$1[number]>, InferOutput<TOptions$1[number]>, UnionIssue<InferIssue<TOptions$1[number]>> | InferIssue<TOptions$1[number]>> {
  /**
   * The schema type.
   */
  readonly type: "union";
  /**
   * The schema reference.
   */
  readonly reference: typeof union;
  /**
   * The union options.
   */
  readonly options: TOptions$1;
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates an union schema.
 *
 * @param options The union options.
 *
 * @returns An union schema.
 */
declare function union<const TOptions$1 extends UnionOptions>(options: TOptions$1): UnionSchema<TOptions$1, undefined>;
/**
 * Creates an union schema.
 *
 * @param options The union options.
 * @param message The error message.
 *
 * @returns An union schema.
 */
declare function union<const TOptions$1 extends UnionOptions, const TMessage extends ErrorMessage<UnionIssue<InferIssue<TOptions$1[number]>>> | undefined>(options: TOptions$1, message: TMessage): UnionSchema<TOptions$1, TMessage>;
//#endregion
//#region src/schemas/nullable/types.d.ts
/**
 * Infer nullable output type.
 */
type InferNullableOutput<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TDefault extends DefaultAsync<TWrapped$1, null>> = undefined extends TDefault ? InferOutput<TWrapped$1> | null : InferOutput<TWrapped$1> | Extract<DefaultValue<TDefault>, null>;
//#endregion
//#region src/schemas/nullable/nullable.d.ts
/**
 * Nullable schema interface.
 */
interface NullableSchema<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, TDefault extends Default<TWrapped$1, null>> extends BaseSchema<InferInput<TWrapped$1> | null, InferNullableOutput<TWrapped$1, TDefault>, InferIssue<TWrapped$1>> {
  /**
   * The schema type.
   */
  readonly type: "nullable";
  /**
   * The schema reference.
   */
  readonly reference: typeof nullable;
  /**
   * The expected property.
   */
  readonly expects: `(${TWrapped$1["expects"]} | null)`;
  /**
   * The wrapped schema.
   */
  readonly wrapped: TWrapped$1;
  /**
   * The default value.
   */
  readonly default: TDefault;
}
/**
 * Creates a nullable schema.
 *
 * @param wrapped The wrapped schema.
 *
 * @returns A nullable schema.
 */
declare function nullable<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>>(wrapped: TWrapped$1): NullableSchema<TWrapped$1, undefined>;
/**
 * Creates a nullable schema.
 *
 * @param wrapped The wrapped schema.
 * @param default_ The default value.
 *
 * @returns A nullable schema.
 */
declare function nullable<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, const TDefault extends Default<TWrapped$1, null>>(wrapped: TWrapped$1, default_: TDefault): NullableSchema<TWrapped$1, TDefault>;
//#endregion
//#region src/schemas/nullish/types.d.ts
/**
 * Infer nullish output type.
 */
type InferNullishOutput<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TDefault extends DefaultAsync<TWrapped$1, null | undefined>> = undefined extends TDefault ? InferOutput<TWrapped$1> | null | undefined : InferOutput<TWrapped$1> | Extract<DefaultValue<TDefault>, null | undefined>;
//#endregion
//#region src/schemas/nullish/nullish.d.ts
/**
 * Nullish schema interface.
 */
interface NullishSchema<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, TDefault extends Default<TWrapped$1, null | undefined>> extends BaseSchema<InferInput<TWrapped$1> | null | undefined, InferNullishOutput<TWrapped$1, TDefault>, InferIssue<TWrapped$1>> {
  /**
   * The schema type.
   */
  readonly type: "nullish";
  /**
   * The schema reference.
   */
  readonly reference: typeof nullish;
  /**
   * The expected property.
   */
  readonly expects: `(${TWrapped$1["expects"]} | null | undefined)`;
  /**
   * The wrapped schema.
   */
  readonly wrapped: TWrapped$1;
  /**
   * The default value.
   */
  readonly default: TDefault;
}
/**
 * Creates a nullish schema.
 *
 * @param wrapped The wrapped schema.
 *
 * @returns A nullish schema.
 */
declare function nullish<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>>(wrapped: TWrapped$1): NullishSchema<TWrapped$1, undefined>;
/**
 * Creates a nullish schema.
 *
 * @param wrapped The wrapped schema.
 * @param default_ The default value.
 *
 * @returns A nullish schema.
 */
declare function nullish<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, const TDefault extends Default<TWrapped$1, null | undefined>>(wrapped: TWrapped$1, default_: TDefault): NullishSchema<TWrapped$1, TDefault>;
//#endregion
//#region src/schemas/nullish/nullishAsync.d.ts
/**
 * Nullish schema async interface.
 */
interface NullishSchemaAsync<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TDefault extends DefaultAsync<TWrapped$1, null | undefined>> extends BaseSchemaAsync<InferInput<TWrapped$1> | null | undefined, InferNullishOutput<TWrapped$1, TDefault>, InferIssue<TWrapped$1>> {
  /**
   * The schema type.
   */
  readonly type: "nullish";
  /**
   * The schema reference.
   */
  readonly reference: typeof nullish | typeof nullishAsync;
  /**
   * The expected property.
   */
  readonly expects: `(${TWrapped$1["expects"]} | null | undefined)`;
  /**
   * The wrapped schema.
   */
  readonly wrapped: TWrapped$1;
  /**
   * The default value.
   */
  readonly default: TDefault;
}
/**
 * Creates a nullish schema.
 *
 * @param wrapped The wrapped schema.
 *
 * @returns A nullish schema.
 */
declare function nullishAsync<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>>(wrapped: TWrapped$1): NullishSchemaAsync<TWrapped$1, undefined>;
/**
 * Creates a nullish schema.
 *
 * @param wrapped The wrapped schema.
 * @param default_ The default value.
 *
 * @returns A nullish schema.
 */
declare function nullishAsync<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, const TDefault extends DefaultAsync<TWrapped$1, null | undefined>>(wrapped: TWrapped$1, default_: TDefault): NullishSchemaAsync<TWrapped$1, TDefault>;
//#endregion
//#region src/schemas/number/number.d.ts
/**
 * Number issue interface.
 */
interface NumberIssue extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "number";
  /**
   * The expected property.
   */
  readonly expected: "number";
}
/**
 * Number schema interface.
 */
interface NumberSchema<TMessage extends ErrorMessage<NumberIssue> | undefined> extends BaseSchema<number, number, NumberIssue> {
  /**
   * The schema type.
   */
  readonly type: "number";
  /**
   * The schema reference.
   */
  readonly reference: typeof number;
  /**
   * The expected property.
   */
  readonly expects: "number";
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates a number schema.
 *
 * @returns A number schema.
 */
declare function number(): NumberSchema<undefined>;
/**
 * Creates a number schema.
 *
 * @param message The error message.
 *
 * @returns A number schema.
 */
declare function number<const TMessage extends ErrorMessage<NumberIssue> | undefined>(message: TMessage): NumberSchema<TMessage>;
//#endregion
//#region src/schemas/object/types.d.ts
/**
 * Object issue interface.
 */
interface ObjectIssue extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "object";
  /**
   * The expected property.
   */
  readonly expected: "Object" | `"${string}"`;
}
//#endregion
//#region src/schemas/object/object.d.ts
/**
 * Object schema interface.
 */
interface ObjectSchema<TEntries$1 extends ObjectEntries, TMessage extends ErrorMessage<ObjectIssue> | undefined> extends BaseSchema<InferObjectInput<TEntries$1>, InferObjectOutput<TEntries$1>, ObjectIssue | InferObjectIssue<TEntries$1>> {
  /**
   * The schema type.
   */
  readonly type: "object";
  /**
   * The schema reference.
   */
  readonly reference: typeof object;
  /**
   * The expected property.
   */
  readonly expects: "Object";
  /**
   * The entries schema.
   */
  readonly entries: TEntries$1;
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates an object schema.
 *
 * Hint: This schema removes unknown entries. The output will only include the
 * entries you specify. To include unknown entries, use `looseObject`. To
 * return an issue for unknown entries, use `strictObject`. To include and
 * validate unknown entries, use `objectWithRest`.
 *
 * @param entries The entries schema.
 *
 * @returns An object schema.
 */
declare function object<const TEntries$1 extends ObjectEntries>(entries: TEntries$1): ObjectSchema<TEntries$1, undefined>;
/**
 * Creates an object schema.
 *
 * Hint: This schema removes unknown entries. The output will only include the
 * entries you specify. To include unknown entries, use `looseObject`. To
 * return an issue for unknown entries, use `strictObject`. To include and
 * validate unknown entries, use `objectWithRest`.
 *
 * @param entries The entries schema.
 * @param message The error message.
 *
 * @returns An object schema.
 */
declare function object<const TEntries$1 extends ObjectEntries, const TMessage extends ErrorMessage<ObjectIssue> | undefined>(entries: TEntries$1, message: TMessage): ObjectSchema<TEntries$1, TMessage>;
//#endregion
//#region src/schemas/optional/types.d.ts
/**
 * Infer optional output type.
 */
type InferOptionalOutput<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TDefault extends DefaultAsync<TWrapped$1, undefined>> = undefined extends TDefault ? InferOutput<TWrapped$1> | undefined : InferOutput<TWrapped$1> | Extract<DefaultValue<TDefault>, undefined>;
//#endregion
//#region src/schemas/optional/optional.d.ts
/**
 * Optional schema interface.
 */
interface OptionalSchema<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, TDefault extends Default<TWrapped$1, undefined>> extends BaseSchema<InferInput<TWrapped$1> | undefined, InferOptionalOutput<TWrapped$1, TDefault>, InferIssue<TWrapped$1>> {
  /**
   * The schema type.
   */
  readonly type: "optional";
  /**
   * The schema reference.
   */
  readonly reference: typeof optional;
  /**
   * The expected property.
   */
  readonly expects: `(${TWrapped$1["expects"]} | undefined)`;
  /**
   * The wrapped schema.
   */
  readonly wrapped: TWrapped$1;
  /**
   * The default value.
   */
  readonly default: TDefault;
}
/**
 * Creates an optional schema.
 *
 * @param wrapped The wrapped schema.
 *
 * @returns An optional schema.
 */
declare function optional<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>>(wrapped: TWrapped$1): OptionalSchema<TWrapped$1, undefined>;
/**
 * Creates an optional schema.
 *
 * @param wrapped The wrapped schema.
 * @param default_ The default value.
 *
 * @returns An optional schema.
 */
declare function optional<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, const TDefault extends Default<TWrapped$1, undefined>>(wrapped: TWrapped$1, default_: TDefault): OptionalSchema<TWrapped$1, TDefault>;
//#endregion
//#region src/schemas/optional/optionalAsync.d.ts
/**
 * Optional schema async interface.
 */
interface OptionalSchemaAsync<TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TDefault extends DefaultAsync<TWrapped$1, undefined>> extends BaseSchemaAsync<InferInput<TWrapped$1> | undefined, InferOptionalOutput<TWrapped$1, TDefault>, InferIssue<TWrapped$1>> {
  /**
   * The schema type.
   */
  readonly type: "optional";
  /**
   * The schema reference.
   */
  readonly reference: typeof optional | typeof optionalAsync;
  /**
   * The expected property.
   */
  readonly expects: `(${TWrapped$1["expects"]} | undefined)`;
  /**
   * The wrapped schema.
   */
  readonly wrapped: TWrapped$1;
  /**
   * The default value.
   */
  readonly default: TDefault;
}
/**
 * Creates an optional schema.
 *
 * @param wrapped The wrapped schema.
 *
 * @returns An optional schema.
 */
declare function optionalAsync<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>>(wrapped: TWrapped$1): OptionalSchemaAsync<TWrapped$1, undefined>;
/**
 * Creates an optional schema.
 *
 * @param wrapped The wrapped schema.
 * @param default_ The default value.
 *
 * @returns An optional schema.
 */
declare function optionalAsync<const TWrapped$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, const TDefault extends DefaultAsync<TWrapped$1, undefined>>(wrapped: TWrapped$1, default_: TDefault): OptionalSchemaAsync<TWrapped$1, TDefault>;
//#endregion
//#region src/schemas/record/types.d.ts
/**
 * Record issue interface.
 */
interface RecordIssue extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "record";
  /**
   * The expected property.
   */
  readonly expected: "Object";
}
/**
 * Is literal type.
 */
type IsLiteral<TKey$1 extends string | number | symbol> = string extends TKey$1 ? false : number extends TKey$1 ? false : symbol extends TKey$1 ? false : TKey$1 extends Brand<string | number | symbol> ? false : true;
/**
 * Optional keys type.
 */
type OptionalKeys<TObject extends Record<string | number | symbol, unknown>> = { [TKey in keyof TObject]: IsLiteral<TKey> extends true ? TKey : never; }[keyof TObject];
/**
 * With question marks type.
 *
 * Hint: We mark an entry as optional if we detect that its key is a literal
 * type. The reason for this is that it is not technically possible to detect
 * missing literal keys without restricting the key schema to `string`, `enum`
 * and `picklist`. However, if `enum` and `picklist` are used, it is better to
 * use `object` with `entriesFromList` because it already covers the needed
 * functionality. This decision also reduces the bundle size of `record`,
 * because it only needs to check the entries of the input and not any missing
 * keys.
 */
type WithQuestionMarks<TObject extends Record<string | number | symbol, unknown>> = MarkOptional<TObject, OptionalKeys<TObject>>;
/**
 * With readonly type.
 */
type WithReadonly<TValue$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>, TObject extends WithQuestionMarks<Record<string | number | symbol, unknown>>> = TValue$1 extends {
  readonly pipe: readonly unknown[];
} ? ReadonlyAction<any> extends TValue$1["pipe"][number] ? Readonly<TObject> : TObject : TObject;
/**
 * Infer record input type.
 */
type InferRecordInput<TKey$1 extends BaseSchema<string, string | number | symbol, BaseIssue<unknown>> | BaseSchemaAsync<string, string | number | symbol, BaseIssue<unknown>>, TValue$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>> = Prettify<WithQuestionMarks<Record<InferInput<TKey$1>, InferInput<TValue$1>>>>;
/**
 * Infer record output type.
 */
type InferRecordOutput<TKey$1 extends BaseSchema<string, string | number | symbol, BaseIssue<unknown>> | BaseSchemaAsync<string, string | number | symbol, BaseIssue<unknown>>, TValue$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>> | BaseSchemaAsync<unknown, unknown, BaseIssue<unknown>>> = Prettify<WithReadonly<TValue$1, WithQuestionMarks<Record<InferOutput<TKey$1>, InferOutput<TValue$1>>>>>;
//#endregion
//#region src/schemas/record/record.d.ts
/**
 * Record schema interface.
 */
interface RecordSchema<TKey$1 extends BaseSchema<string, string | number | symbol, BaseIssue<unknown>>, TValue$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, TMessage extends ErrorMessage<RecordIssue> | undefined> extends BaseSchema<InferRecordInput<TKey$1, TValue$1>, InferRecordOutput<TKey$1, TValue$1>, RecordIssue | InferIssue<TKey$1> | InferIssue<TValue$1>> {
  /**
   * The schema type.
   */
  readonly type: "record";
  /**
   * The schema reference.
   */
  readonly reference: typeof record;
  /**
   * The expected property.
   */
  readonly expects: "Object";
  /**
   * The record key schema.
   */
  readonly key: TKey$1;
  /**
   * The record value schema.
   */
  readonly value: TValue$1;
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates a record schema.
 *
 * @param key The key schema.
 * @param value The value schema.
 *
 * @returns A record schema.
 */
declare function record<const TKey$1 extends BaseSchema<string, string | number | symbol, BaseIssue<unknown>>, const TValue$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>>(key: TKey$1, value: TValue$1): RecordSchema<TKey$1, TValue$1, undefined>;
/**
 * Creates a record schema.
 *
 * @param key The key schema.
 * @param value The value schema.
 * @param message The error message.
 *
 * @returns A record schema.
 */
declare function record<const TKey$1 extends BaseSchema<string, string | number | symbol, BaseIssue<unknown>>, const TValue$1 extends BaseSchema<unknown, unknown, BaseIssue<unknown>>, const TMessage extends ErrorMessage<RecordIssue> | undefined>(key: TKey$1, value: TValue$1, message: TMessage): RecordSchema<TKey$1, TValue$1, TMessage>;
//#endregion
//#region src/schemas/string/string.d.ts
/**
 * String issue interface.
 */
interface StringIssue extends BaseIssue<unknown> {
  /**
   * The issue kind.
   */
  readonly kind: "schema";
  /**
   * The issue type.
   */
  readonly type: "string";
  /**
   * The expected property.
   */
  readonly expected: "string";
}
/**
 * String schema interface.
 */
interface StringSchema<TMessage extends ErrorMessage<StringIssue> | undefined> extends BaseSchema<string, string, StringIssue> {
  /**
   * The schema type.
   */
  readonly type: "string";
  /**
   * The schema reference.
   */
  readonly reference: typeof string;
  /**
   * The expected property.
   */
  readonly expects: "string";
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates a string schema.
 *
 * @returns A string schema.
 */
declare function string(): StringSchema<undefined>;
/**
 * Creates a string schema.
 *
 * @param message The error message.
 *
 * @returns A string schema.
 */
declare function string<const TMessage extends ErrorMessage<StringIssue> | undefined>(message: TMessage): StringSchema<TMessage>;
//#endregion
//#region src/schemas/unknown/unknown.d.ts
/**
 * Unknown schema interface.
 */
interface UnknownSchema extends BaseSchema<unknown, unknown, never> {
  /**
   * The schema type.
   */
  readonly type: "unknown";
  /**
   * The schema reference.
   */
  readonly reference: typeof unknown;
  /**
   * The expected property.
   */
  readonly expects: "unknown";
}
/**
 * Creates a unknown schema.
 *
 * @returns A unknown schema.
 */
declare function unknown(): UnknownSchema;
//#endregion
//#region src/actions/brand/brand.d.ts
/**
 * Brand symbol.
 */
declare const BrandSymbol: unique symbol;
/**
 * Brand name type.
 */
type BrandName = string | number | symbol;
/**
 * Brand interface.
 */
interface Brand<TName extends BrandName> {
  [BrandSymbol]: { [TValue in TName]: TValue; };
}
//#endregion
//#region src/actions/check/types.d.ts
/**
 * Check issue interface.
 */
interface CheckIssue<TInput$1> extends BaseIssue<TInput$1> {
  /**
   * The issue kind.
   */
  readonly kind: "validation";
  /**
   * The issue type.
   */
  readonly type: "check";
  /**
   * The expected property.
   */
  readonly expected: null;
  /**
   * The validation function.
   */
  readonly requirement: (input: TInput$1) => MaybePromise<boolean>;
}
//#endregion
//#region src/actions/check/check.d.ts
/**
 * Check action interface.
 */
interface CheckAction<TInput$1, TMessage extends ErrorMessage<CheckIssue<TInput$1>> | undefined> extends BaseValidation<TInput$1, TInput$1, CheckIssue<TInput$1>> {
  /**
   * The action type.
   */
  readonly type: "check";
  /**
   * The action reference.
   */
  readonly reference: typeof check;
  /**
   * The expected property.
   */
  readonly expects: null;
  /**
   * The validation function.
   */
  readonly requirement: (input: TInput$1) => boolean;
  /**
   * The error message.
   */
  readonly message: TMessage;
}
/**
 * Creates a check validation action.
 *
 * @param requirement The validation function.
 *
 * @returns A check action.
 */
declare function check<TInput$1>(requirement: (input: TInput$1) => boolean): CheckAction<TInput$1, undefined>;
/**
 * Creates a check validation action.
 *
 * @param requirement The validation function.
 * @param message The error message.
 *
 * @returns A check action.
 */
declare function check<TInput$1, const TMessage extends ErrorMessage<CheckIssue<TInput$1>> | undefined>(requirement: (input: TInput$1) => boolean, message: TMessage): CheckAction<TInput$1, TMessage>;
//#endregion
//#region src/actions/readonly/readonly.d.ts
/**
 * Readonly output type.
 */
type ReadonlyOutput<TInput$1> = TInput$1 extends Map<infer TKey, infer TValue> ? ReadonlyMap<TKey, TValue> : TInput$1 extends Set<infer TValue> ? ReadonlySet<TValue> : Readonly<TInput$1>;
/**
 * Readonly action interface.
 */
interface ReadonlyAction<TInput$1> extends BaseTransformation<TInput$1, ReadonlyOutput<TInput$1>, never> {
  /**
   * The action type.
   */
  readonly type: "readonly";
  /**
   * The action reference.
   */
  readonly reference: typeof readonly;
}
/**
 * Creates a readonly transformation action.
 *
 * @returns A readonly action.
 */
declare function readonly<TInput$1>(): ReadonlyAction<TInput$1>;
//#endregion
//#region src/actions/transform/transform.d.ts
/**
 * Transform action interface.
 */
interface TransformAction<TInput$1, TOutput$1> extends BaseTransformation<TInput$1, TOutput$1, never> {
  /**
   * The action type.
   */
  readonly type: "transform";
  /**
   * The action reference.
   */
  readonly reference: typeof transform;
  /**
   * The transformation operation.
   */
  readonly operation: (input: TInput$1) => TOutput$1;
}
/**
 * Creates a custom transformation action.
 *
 * @param operation The transformation operation.
 *
 * @returns A transform action.
 */
declare function transform<TInput$1, TOutput$1>(operation: (input: TInput$1) => TOutput$1): TransformAction<TInput$1, TOutput$1>;
//#endregion
//#region src/@types/options.d.ts
declare const ZInputFormatType: UnionSchema<[LiteralSchema<"XMLDocument", undefined>, LiteralSchema<"niconicome", undefined>, LiteralSchema<"xml2js", undefined>, LiteralSchema<"formatted", undefined>, LiteralSchema<"legacy", undefined>, LiteralSchema<"legacyOwner", undefined>, LiteralSchema<"owner", undefined>, LiteralSchema<"v1", undefined>, LiteralSchema<"empty", undefined>, LiteralSchema<"default", undefined>], undefined>;
type InputFormatType = InferOutput<typeof ZInputFormatType>;
type InputFormat = XMLDocument | Xml2jsPacket | FormattedCommentInput[] | FormattedLegacyCommentInput[] | RawApiResponse[] | OwnerComment[] | V1Thread[] | string | undefined;
type ModeType = "default" | "html5" | "flash";
type BaseOptions = {
  config: Config;
  debug: boolean;
  enableLegacyPiP: boolean;
  format: InputFormatType;
  formatted: boolean;
  keepCA: boolean;
  mode: ModeType;
  scale: number;
  showCollision: boolean;
  showCommentCount: boolean;
  showFPS: boolean;
  useLegacy: boolean;
  video: HTMLVideoElement | undefined;
  lazy: boolean;
};
type Options = Partial<BaseOptions>;
type inputFormatType = InputFormatType;
type inputFormat = InputFormat;
//#endregion
//#region src/@types/config.d.ts
type ConfigItem<T> = T | MultiConfigItem<T>;
type MultiConfigItem<T> = {
  html5: T;
  flash: T;
};
type ConfigSizeItem<T> = {
  big: T;
  medium: T;
  small: T;
};
type ConfigResizedItem<T> = {
  default: T;
  resized: T;
};
type ConfigFlashFontItem<T> = {
  gulim: T;
  simsun: T;
  defont: T;
};
type ConfigHTML5FontItem<T> = {
  gothic: T;
  mincho: T;
  defont: T;
};
type CommentStageSize = {
  width: number;
  fullWidth: number;
  height: number;
};
type FlashCharList = { [key in "simsunStrong" | "simsunWeak" | "gulim" | "gothic"]: string; };
type FlashMode = "xp" | "vista";
type FlashScriptChar = { [key in "super" | "sub"]: string; };
type FontList = { [key in "gulim" | "simsun"]: string; };
type LineCounts = { [key in "default" | "resized" | "doubleResized"]: ConfigSizeItem<number>; };
type BaseConfig = {
  cacheAge: number;
  canvasHeight: number;
  canvasWidth: number;
  collisionRange: { [key in "left" | "right"]: number; };
  collisionPadding: number;
  colors: {
    [key: string]: string;
  };
  commentDrawPadding: number;
  commentDrawRange: number;
  commentScale: ConfigItem<number>;
  commentStageSize: ConfigItem<CommentStageSize>;
  flashCommentYOffset: ConfigSizeItem<ConfigResizedItem<number>>;
  flashCommentYPaddingTop: ConfigResizedItem<number>;
  contextFillLiveOpacity: number;
  contextLineWidth: ConfigItem<number>;
  contextStrokeColor: string;
  contextStrokeInversionColor: string;
  contextStrokeOpacity: number;
  flashChar: FlashCharList;
  flashMode: FlashMode;
  flashScriptChar: FlashScriptChar;
  flashThreshold: number;
  fonts: {
    flash: FontList;
    html5: PlatformFont;
  };
  fontSize: ConfigItem<ConfigSizeItem<ConfigResizedItem<number>>>;
  fpsInterval: number;
  html5HiResCommentCorrection: number;
  html5LineCounts: ConfigItem<LineCounts>;
  lineHeight: ConfigItem<ConfigSizeItem<ConfigResizedItem<number>>>;
  html5MinFontSize: number;
  sameCAGap: number;
  sameCAMinScore: number;
  sameCARange: number;
  sameCATimestampRange: number;
  flashLetterSpacing: number;
  flashScriptCharOffset: number;
  plugins: IPluginConstructor[];
  commentPlugins: {
    class: typeof BaseComment;
    condition: (comment: FormattedComment, config: BaseConfig, options: BaseOptions) => boolean;
  }[];
  commentLimit: number | undefined;
  hideCommentOrder: "asc" | "desc";
  lineBreakCount: { [key in CommentSize]: number; };
  nakaCommentSpeedOffset: number;
  atButtonPadding: number;
  atButtonRadius: number;
  flashDoubleResizeHeights: Partial<ConfigSizeItem<{
    [key: number]: number;
  }>>;
  flashLineBreakScale: ConfigSizeItem<number>;
  compatSpacer: {
    flash: {
      [key: string]: Partial<ConfigFlashFontItem<number>>;
    };
    html5: {
      [key: string]: Partial<ConfigHTML5FontItem<number>>;
    };
  };
};
type Config = Partial<BaseConfig>;
//#endregion
//#region src/@types/cursor.d.ts
type Position = {
  x: number;
  y: number;
};
//#endregion
//#region src/@types/event.d.ts
interface CommentEventBase {
  type: CommentEventName;
  timeStamp: number;
  vpos: number;
}
interface SeekDisableEvent extends CommentEventBase {
  type: "seekDisable";
}
type SeekDisableEventHandler = (event: SeekDisableEvent) => unknown;
interface SeekEnableEvent extends CommentEventBase {
  type: "seekEnable";
}
type SeekEnableEventHandler = (event: SeekEnableEvent) => unknown;
interface CommentDisableEvent extends CommentEventBase {
  type: "commentDisable";
}
type CommentDisableEventHandler = (event: CommentDisableEvent) => unknown;
interface CommentEnableEvent extends CommentEventBase {
  type: "commentEnable";
}
type CommentEnableEventHandler = (event: CommentEnableEvent) => unknown;
interface JumpEvent extends CommentEventBase {
  type: "jump";
  to: string;
  message?: string;
}
type JumpEventHandler = (event: JumpEvent) => unknown;
type CommentEventName = "seekDisable" | "seekEnable" | "commentDisable" | "commentEnable" | "jump";
type CommentEventHandler = SeekDisableEventHandler | SeekEnableEventHandler | CommentDisableEventHandler | CommentEnableEventHandler | JumpEventHandler;
interface CommentEventHandlerMap {
  seekDisable: SeekDisableEventHandler;
  seekEnable: SeekEnableEventHandler;
  commentDisable: CommentDisableEventHandler;
  commentEnable: CommentEnableEventHandler;
  jump: JumpEventHandler;
}
interface CommentEventMap {
  seekDisable: SeekDisableEvent;
  seekEnable: SeekEnableEvent;
  commentDisable: CommentDisableEvent;
  commentEnable: CommentEnableEvent;
  jump: JumpEvent;
}
//#endregion
//#region src/@types/fonts.d.ts
type Platform = "win7" | "win8_1" | "win" | "mac10_9" | "mac10_11" | "mac" | "other";
declare const ZHTML5Fonts: UnionSchema<[LiteralSchema<"gothic", undefined>, LiteralSchema<"mincho", undefined>, LiteralSchema<"defont", undefined>], undefined>;
type HTML5Fonts = InferOutput<typeof ZHTML5Fonts>;
type FontItem = {
  font: string;
  offset: number;
  weight: number;
};
type PlatformFont = { [key in HTML5Fonts]: FontItem; };
//#endregion
//#region src/@types/format.formatted.d.ts
declare const ZFormattedComment: SchemaWithPipe<readonly [ObjectSchema<{
  readonly id: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly vpos: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly content: OptionalSchema<StringSchema<undefined>, "">;
  readonly date: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly date_usec: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly owner: OptionalSchema<BooleanSchema<undefined>, false>;
  readonly premium: OptionalSchema<BooleanSchema<undefined>, false>;
  readonly mail: OptionalSchema<ArraySchema<StringSchema<undefined>, undefined>, readonly []>;
  readonly user_id: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly layer: OptionalSchema<UnionSchema<[LiteralSchema<-1, undefined>, LiteralSchema<-2, undefined>, SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>], undefined>, undefined>;
  readonly ignoreScale: OptionalSchema<BooleanSchema<undefined>, false>;
  readonly is_my_post: OptionalSchema<BooleanSchema<undefined>, false>;
}, undefined>, TransformAction<{
  id: number;
  vpos: number;
  content: string;
  date: number;
  date_usec: number;
  owner: boolean;
  premium: boolean;
  mail: string[];
  user_id: number;
  layer?: number | undefined;
  ignoreScale: boolean;
  is_my_post: boolean;
}, {
  layer: number;
  id: number;
  vpos: number;
  content: string;
  date: number;
  date_usec: number;
  owner: boolean;
  premium: boolean;
  mail: string[];
  user_id: number;
  ignoreScale: boolean;
  is_my_post: boolean;
}>]>;
type FormattedComment = InferOutput<typeof ZFormattedComment>;
type FormattedCommentInput = InferInput<typeof ZFormattedComment>;
declare const ZFormattedLegacyComment: Omit<ObjectSchema<{
  readonly id: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly vpos: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly content: OptionalSchema<StringSchema<undefined>, "">;
  readonly date: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly date_usec: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly owner: OptionalSchema<BooleanSchema<undefined>, false>;
  readonly premium: OptionalSchema<BooleanSchema<undefined>, false>;
  readonly mail: OptionalSchema<ArraySchema<StringSchema<undefined>, undefined>, readonly []>;
  readonly user_id: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly layer: OptionalSchema<UnionSchema<[LiteralSchema<-1, undefined>, LiteralSchema<-2, undefined>, SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>], undefined>, undefined>;
  readonly ignoreScale: OptionalSchema<BooleanSchema<undefined>, false>;
  readonly is_my_post: OptionalSchema<BooleanSchema<undefined>, false>;
}, undefined>, "~types" | "~run" | "~standard" | "entries"> & {
  readonly entries: Omit<{
    readonly id: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
    readonly vpos: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
    readonly content: OptionalSchema<StringSchema<undefined>, "">;
    readonly date: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
    readonly date_usec: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
    readonly owner: OptionalSchema<BooleanSchema<undefined>, false>;
    readonly premium: OptionalSchema<BooleanSchema<undefined>, false>;
    readonly mail: OptionalSchema<ArraySchema<StringSchema<undefined>, undefined>, readonly []>;
    readonly user_id: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
    readonly layer: OptionalSchema<UnionSchema<[LiteralSchema<-1, undefined>, LiteralSchema<-2, undefined>, SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>], undefined>, undefined>;
    readonly ignoreScale: OptionalSchema<BooleanSchema<undefined>, false>;
    readonly is_my_post: OptionalSchema<BooleanSchema<undefined>, false>;
  }, "user_id" | "layer" | "ignoreScale" | "is_my_post">;
  readonly "~standard": StandardProps<{
    id?: number | undefined;
    vpos?: number | undefined;
    content?: string | undefined;
    date?: number | undefined;
    date_usec?: number | undefined;
    owner?: boolean | undefined;
    premium?: boolean | undefined;
    mail?: string[] | undefined;
  }, {
    id: number;
    vpos: number;
    content: string;
    date: number;
    date_usec: number;
    owner: boolean;
    premium: boolean;
    mail: string[];
  }>;
  readonly "~run": (dataset: UnknownDataset, config: Config$1<BaseIssue<unknown>>) => OutputDataset<{
    id: number;
    vpos: number;
    content: string;
    date: number;
    date_usec: number;
    owner: boolean;
    premium: boolean;
    mail: string[];
  }, NumberIssue | CheckIssue<number> | StringIssue | BooleanIssue | ArrayIssue | ObjectIssue>;
  readonly "~types"?: {
    readonly input: {
      id?: number | undefined;
      vpos?: number | undefined;
      content?: string | undefined;
      date?: number | undefined;
      date_usec?: number | undefined;
      owner?: boolean | undefined;
      premium?: boolean | undefined;
      mail?: string[] | undefined;
    };
    readonly output: {
      id: number;
      vpos: number;
      content: string;
      date: number;
      date_usec: number;
      owner: boolean;
      premium: boolean;
      mail: string[];
    };
    readonly issue: NumberIssue | CheckIssue<number> | StringIssue | BooleanIssue | ArrayIssue | ObjectIssue;
  } | undefined;
};
type FormattedLegacyComment = InferOutput<typeof ZFormattedLegacyComment>;
type FormattedLegacyCommentInput = InferInput<typeof ZFormattedLegacyComment>;
type formattedComment = FormattedComment;
type formattedLegacyComment = FormattedLegacyComment;
//#endregion
//#region src/@types/format.legacy.d.ts
declare const ZApiChat: ObjectSchema<{
  readonly thread: OptionalSchema<StringSchema<undefined>, "">;
  readonly no: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly vpos: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
  readonly date: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly date_usec: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly nicoru: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly premium: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly anonymity: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly user_id: OptionalSchema<StringSchema<undefined>, "">;
  readonly mail: OptionalSchema<StringSchema<undefined>, "">;
  readonly content: StringSchema<undefined>;
  readonly deleted: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
}, undefined>;
type ApiChat = InferOutput<typeof ZApiChat>;
declare const ZRawApiResponse: RecordSchema<StringSchema<undefined>, UnknownSchema, undefined>;
type RawApiResponse = InferOutput<typeof ZRawApiResponse>;
declare const ZApiPing: ObjectSchema<{
  readonly content: StringSchema<undefined>;
}, undefined>;
type ApiPing = InferOutput<typeof ZApiPing>;
declare const ZApiThread: ObjectSchema<{
  readonly resultcode: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
  readonly thread: StringSchema<undefined>;
  readonly server_time: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
  readonly ticket: StringSchema<undefined>;
  readonly revision: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
}, undefined>;
type ApiThread = InferOutput<typeof ZApiThread>;
declare const ZApiLeaf: ObjectSchema<{
  readonly thread: StringSchema<undefined>;
  readonly count: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
}, undefined>;
type ApiLeaf = InferOutput<typeof ZApiLeaf>;
declare const ZApiGlobalNumRes: ObjectSchema<{
  readonly thread: StringSchema<undefined>;
  readonly num_res: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
}, undefined>;
type ApiGlobalNumRes = InferOutput<typeof ZApiGlobalNumRes>;
type rawApiResponse = RawApiResponse;
//#endregion
//#region src/@types/format.numeric.d.ts
type NumberRangeOptions = {
  min?: number;
  max?: number;
  integer?: boolean;
};
declare const isFiniteNumberInRange: (value: unknown, { min, max, integer }?: NumberRangeOptions) => value is number;
declare const ZCommentId: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
declare const ZCommentVpos: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
declare const ZCommentDate: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
declare const ZCommentDateUsec: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
declare const ZCommentUserId: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
declare const VIEWER_DEFAULT_COLLISION_LAYER = -1;
declare const OWNER_DEFAULT_COLLISION_LAYER = -2;
declare const getDefaultCollisionLayer: (owner: boolean) => -1 | -2;
declare const ZCommentLayer: UnionSchema<[LiteralSchema<-1, undefined>, LiteralSchema<-2, undefined>, SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>], undefined>;
declare const ZCommentScore: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
declare const toFiniteNumberInRange: (value: unknown, options?: NumberRangeOptions) => number | undefined;
//#endregion
//#region src/@types/format.owner.d.ts
declare const ZOwnerComment: ObjectSchema<{
  readonly time: StringSchema<undefined>;
  readonly command: OptionalSchema<StringSchema<undefined>, "">;
  readonly comment: StringSchema<undefined>;
}, undefined>;
type OwnerComment = InferOutput<typeof ZOwnerComment>;
type ownerComment = OwnerComment;
//#endregion
//#region src/@types/format.v1.d.ts
declare const ZV1Comment: ObjectSchema<{
  readonly id: StringSchema<undefined>;
  readonly no: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
  readonly vposMs: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
  readonly body: StringSchema<undefined>;
  readonly commands: ArraySchema<StringSchema<undefined>, undefined>;
  readonly userId: StringSchema<undefined>;
  readonly isPremium: BooleanSchema<undefined>;
  readonly score: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
  readonly postedAt: StringSchema<undefined>;
  readonly nicoruCount: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
  readonly nicoruId: NullableSchema<StringSchema<undefined>, undefined>;
  readonly source: StringSchema<undefined>;
  readonly isMyPost: BooleanSchema<undefined>;
}, undefined>;
type V1Comment = InferOutput<typeof ZV1Comment>;
declare const ZV1Thread: ObjectSchema<{
  readonly id: UnknownSchema;
  readonly fork: StringSchema<undefined>;
  readonly commentCount: OptionalSchema<SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>, 0>;
  readonly comments: ArraySchema<ObjectSchema<{
    readonly id: StringSchema<undefined>;
    readonly no: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
    readonly vposMs: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
    readonly body: StringSchema<undefined>;
    readonly commands: ArraySchema<StringSchema<undefined>, undefined>;
    readonly userId: StringSchema<undefined>;
    readonly isPremium: BooleanSchema<undefined>;
    readonly score: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
    readonly postedAt: StringSchema<undefined>;
    readonly nicoruCount: SchemaWithPipe<readonly [NumberSchema<undefined>, CheckAction<number, undefined>]>;
    readonly nicoruId: NullableSchema<StringSchema<undefined>, undefined>;
    readonly source: StringSchema<undefined>;
    readonly isMyPost: BooleanSchema<undefined>;
  }, undefined>, undefined>;
}, undefined>;
type V1Thread = InferOutput<typeof ZV1Thread>;
type v1Thread = V1Thread;
//#endregion
//#region src/@types/format.xml2js.d.ts
declare const ZXml2jsChatItem: ObjectSchema<{
  readonly _: OptionalSchema<StringSchema<undefined>, "">;
  readonly $: ObjectSchema<{
    readonly no: OptionalSchema<StringSchema<undefined>, undefined>;
    readonly vpos: StringSchema<undefined>;
    readonly date: OptionalSchema<StringSchema<undefined>, "0">;
    readonly date_usec: OptionalSchema<StringSchema<undefined>, "0">;
    readonly user_id: OptionalSchema<StringSchema<undefined>, undefined>;
    readonly owner: OptionalSchema<StringSchema<undefined>, "">;
    readonly premium: OptionalSchema<StringSchema<undefined>, "">;
    readonly mail: OptionalSchema<StringSchema<undefined>, "">;
  }, undefined>;
}, undefined>;
type Xml2jsChatItem = InferOutput<typeof ZXml2jsChatItem>;
declare const ZXml2jsChat: ObjectSchema<{
  readonly chat: ArraySchema<ObjectSchema<{
    readonly _: OptionalSchema<StringSchema<undefined>, "">;
    readonly $: ObjectSchema<{
      readonly no: OptionalSchema<StringSchema<undefined>, undefined>;
      readonly vpos: StringSchema<undefined>;
      readonly date: OptionalSchema<StringSchema<undefined>, "0">;
      readonly date_usec: OptionalSchema<StringSchema<undefined>, "0">;
      readonly user_id: OptionalSchema<StringSchema<undefined>, undefined>;
      readonly owner: OptionalSchema<StringSchema<undefined>, "">;
      readonly premium: OptionalSchema<StringSchema<undefined>, "">;
      readonly mail: OptionalSchema<StringSchema<undefined>, "">;
    }, undefined>;
  }, undefined>, undefined>;
}, undefined>;
type Xml2jsChat = InferOutput<typeof ZXml2jsChat>;
declare const ZXml2jsPacket: ObjectSchema<{
  readonly packet: ObjectSchema<{
    readonly chat: ArraySchema<ObjectSchema<{
      readonly _: OptionalSchema<StringSchema<undefined>, "">;
      readonly $: ObjectSchema<{
        readonly no: OptionalSchema<StringSchema<undefined>, undefined>;
        readonly vpos: StringSchema<undefined>;
        readonly date: OptionalSchema<StringSchema<undefined>, "0">;
        readonly date_usec: OptionalSchema<StringSchema<undefined>, "0">;
        readonly user_id: OptionalSchema<StringSchema<undefined>, undefined>;
        readonly owner: OptionalSchema<StringSchema<undefined>, "">;
        readonly premium: OptionalSchema<StringSchema<undefined>, "">;
        readonly mail: OptionalSchema<StringSchema<undefined>, "">;
      }, undefined>;
    }, undefined>, undefined>;
  }, undefined>;
}, undefined>;
type Xml2jsPacket = InferOutput<typeof ZXml2jsPacket>;
//#endregion
//#region src/@types/IComment.d.ts
type FrameActiveState = {
  banActive: boolean;
  reverseActiveOwner: boolean;
  reverseActiveViewer: boolean;
};
interface IComment {
  comment: FormattedCommentWithSize;
  invisible: boolean;
  index: number;
  loc: CommentLoc;
  width: number;
  long: number;
  height: number;
  vpos: number;
  flash: boolean;
  posY: number;
  owner: boolean;
  layer: number;
  ignoreScale: boolean;
  mail: string[];
  content: string;
  image?: IRenderer | null;
  draw: (vpos: number, showCollision: boolean, cursor?: Position, frameActiveState?: FrameActiveState) => void;
  destroy?: () => void;
  isHovered: (cursor?: Position, posX?: number, posY?: number) => boolean;
}
//#endregion
//#region src/@types/IPlugins.d.ts
interface IPluginConstructor {
  id?: string;
  new (Canvas: IRenderer, comments: IComment[]): IPlugin;
}
interface IPlugin {
  draw?: (vpos: number) => boolean | undefined;
  addComments?: (comments: IComment[]) => void;
  transformComments?: (comments: IComment[]) => IComment[];
  destroy?: () => void;
}
type IPluginList = {
  instance: IPlugin;
  canvas: IRenderer;
}[];
//#endregion
//#region src/@types/input-parser.d.ts
interface InputParser {
  key: string[];
  parse: (input: unknown) => FormattedComment[];
}
//#endregion
//#region src/@types/renderer.d.ts
interface IRenderer {
  readonly rendererName: string;
  readonly canvas: HTMLCanvasElement;
  destroy(): void;
  drawVideo(enableLegacyPip: boolean): void;
  getFont(): string;
  getFillStyle(): string | CanvasGradient | CanvasPattern;
  setScale(scale: number, arg1?: number): void;
  fillRect(x: number, y: number, width: number, height: number): void;
  strokeRect(x: number, y: number, width: number, height: number): void;
  fillText(text: string, x: number, y: number): void;
  strokeText(text: string, x: number, y: number): void;
  quadraticCurveTo(cpx: number, cpy: number, x: number, y: number): void;
  clearRect(x: number, y: number, width: number, height: number): void;
  setFont(font: string): void;
  setFillStyle(color: string): void;
  setStrokeStyle(color: string): void;
  setLineWidth(width: number): void;
  setGlobalAlpha(alpha: number): void;
  setSize(width: number, height: number): void;
  getSize(): {
    width: number;
    height: number;
  };
  measureText(text: string): TextMetrics;
  measureTextAtDrawScale?(text: string, drawScale: number): TextMetrics;
  beginPath(): void;
  closePath(): void;
  moveTo(x: number, y: number): void;
  lineTo(x: number, y: number): void;
  stroke(): void;
  save(): void;
  restore(): void;
  getCanvas(padding?: number): IRenderer;
  drawImage(image: IRenderer, x: number, y: number, width?: number, height?: number): void;
  flush(): void;
  needsRedraw?(): boolean;
  invalidateImage(image: IRenderer): void;
}
//#endregion
//#region src/@types/types.d.ts
type FormattedCommentWithFont = {
  id: number;
  vpos: number;
  date: number;
  date_usec: number;
  owner: boolean;
  premium: boolean;
  mail: string[];
  user_id: number;
  layer: number;
  ignoreScale: boolean;
  loc: CommentLoc;
  size: CommentSize;
  fontSize: number;
  font: CommentFont;
  color: string;
  strokeColor?: string;
  wakuColor?: string;
  fillColor?: string;
  opacity?: number;
  commandScale?: number;
  full: boolean;
  ender: boolean;
  _live: boolean;
  long: number;
  invisible: boolean;
  content: CommentContentItem[];
  rawContent: string;
  flash: boolean;
  lineCount: number;
  lineOffset: number;
  is_my_post: boolean;
  button?: ButtonParams;
};
type FormattedCommentWithSize = FormattedCommentWithFont & {
  height: number;
  width: number;
  lineHeight: number;
  resized: boolean;
  resizedX: boolean;
  resizedY: boolean;
  content: CommentMeasuredContentItem[];
  charSize: number;
  scale: number;
  scaleX: number;
  layerScale: number;
  buttonObjects?: ButtonList;
};
type ParseContentResult = {
  content: CommentContentItem[];
  lineCount: number;
  lineOffset: number;
};
type ParseCommandAndNicoScriptResult = {
  flash: boolean;
  loc: CommentLoc;
  size: CommentSize;
  fontSize: number;
  color: string;
  strokeColor?: string;
  wakuColor?: string;
  fillColor?: string;
  opacity?: number;
  commandScale?: number;
  ignoreScale: boolean;
  font: CommentFont;
  full: boolean;
  ender: boolean;
  _live: boolean;
  invisible: boolean;
  long: number;
  button?: ButtonParams;
};
declare const ZCommentFont: UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"mincho", undefined>, LiteralSchema<"gothic", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>;
type CommentFont = InferOutput<typeof ZCommentFont>;
declare const ZCommentHTML5Font: UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"mincho", undefined>, LiteralSchema<"gothic", undefined>], undefined>;
type CommentHTML5Font = InferOutput<typeof ZCommentHTML5Font>;
declare const ZCommentFlashFont: UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>;
type CommentFlashFont = InferOutput<typeof ZCommentFlashFont>;
declare const ZCommentContentItemSpacer: ObjectSchema<{
  readonly type: LiteralSchema<"spacer", undefined>;
  readonly char: StringSchema<undefined>;
  readonly charWidth: NumberSchema<undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly count: NumberSchema<undefined>;
}, undefined>;
declare const ZCommentContentItemText: ObjectSchema<{
  readonly type: LiteralSchema<"text", undefined>;
  readonly content: StringSchema<undefined>;
  readonly slicedContent: ArraySchema<StringSchema<undefined>, undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly width: OptionalSchema<ArraySchema<NumberSchema<undefined>, undefined>, undefined>;
}, undefined>;
type CommentContentItemText = InferOutput<typeof ZCommentContentItemText>;
declare const ZCommentContentItem: UnionSchema<[ObjectSchema<{
  readonly type: LiteralSchema<"spacer", undefined>;
  readonly char: StringSchema<undefined>;
  readonly charWidth: NumberSchema<undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly count: NumberSchema<undefined>;
}, undefined>, ObjectSchema<{
  readonly type: LiteralSchema<"text", undefined>;
  readonly content: StringSchema<undefined>;
  readonly slicedContent: ArraySchema<StringSchema<undefined>, undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly width: OptionalSchema<ArraySchema<NumberSchema<undefined>, undefined>, undefined>;
}, undefined>], undefined>;
type CommentContentItem = InferOutput<typeof ZCommentContentItem>;
declare const ZCommentMeasuredContentItemText: IntersectSchema<[UnionSchema<[ObjectSchema<{
  readonly type: LiteralSchema<"spacer", undefined>;
  readonly char: StringSchema<undefined>;
  readonly charWidth: NumberSchema<undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly count: NumberSchema<undefined>;
}, undefined>, ObjectSchema<{
  readonly type: LiteralSchema<"text", undefined>;
  readonly content: StringSchema<undefined>;
  readonly slicedContent: ArraySchema<StringSchema<undefined>, undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly width: OptionalSchema<ArraySchema<NumberSchema<undefined>, undefined>, undefined>;
}, undefined>], undefined>, ObjectSchema<{
  readonly width: ArraySchema<NumberSchema<undefined>, undefined>;
}, undefined>], undefined>;
declare const ZCommentMeasuredContentItem: UnionSchema<[IntersectSchema<[UnionSchema<[ObjectSchema<{
  readonly type: LiteralSchema<"spacer", undefined>;
  readonly char: StringSchema<undefined>;
  readonly charWidth: NumberSchema<undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly count: NumberSchema<undefined>;
}, undefined>, ObjectSchema<{
  readonly type: LiteralSchema<"text", undefined>;
  readonly content: StringSchema<undefined>;
  readonly slicedContent: ArraySchema<StringSchema<undefined>, undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly width: OptionalSchema<ArraySchema<NumberSchema<undefined>, undefined>, undefined>;
}, undefined>], undefined>, ObjectSchema<{
  readonly width: ArraySchema<NumberSchema<undefined>, undefined>;
}, undefined>], undefined>, ObjectSchema<{
  readonly type: LiteralSchema<"spacer", undefined>;
  readonly char: StringSchema<undefined>;
  readonly charWidth: NumberSchema<undefined>;
  readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
  readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
  readonly count: NumberSchema<undefined>;
}, undefined>], undefined>;
type CommentMeasuredContentItem = InferOutput<typeof ZCommentMeasuredContentItem>;
type CommentFlashFontParsed = "gothic" | "gulim" | "simsunStrong" | "simsunWeak";
type CommentContentIndex = {
  index: number;
  font: CommentFlashFontParsed;
};
declare const ZCommentSize: UnionSchema<[LiteralSchema<"big", undefined>, LiteralSchema<"medium", undefined>, LiteralSchema<"small", undefined>], undefined>;
type CommentSize = InferOutput<typeof ZCommentSize>;
declare const ZCommentLoc: UnionSchema<[LiteralSchema<"ue", undefined>, LiteralSchema<"naka", undefined>, LiteralSchema<"shita", undefined>], undefined>;
type CommentLoc = InferOutput<typeof ZCommentLoc>;
type Collision = { [key in CollisionPos]: CollisionItem; };
type Timeline = {
  [key: number]: IComment[];
};
type CollisionPos = "ue" | "shita" | "right" | "left";
type CollisionItem = {
  [p: number]: IComment[];
};
type NicoScript = {
  reverse: NicoScriptReverse[];
  ban: NicoScriptBan[];
  default: NicoScriptDefault[];
  replace: NicoScriptReplace[];
  seekDisable: NicoScriptSeekDisable[];
  jump: NicoScriptJump[];
};
type NicoScriptSeekDisable = {
  start: number;
  end: number;
};
type NicoScriptJump = {
  start: number;
  end?: number;
  to: string;
  message?: string;
};
type NicoScriptReverse = {
  target: NicoScriptReverseTarget;
  start: number;
  end: number;
};
declare const ZNicoScriptReverseTarget: UnionSchema<[LiteralSchema<"コメ", undefined>, LiteralSchema<"投コメ", undefined>, LiteralSchema<"全", undefined>], undefined>;
type NicoScriptReverseTarget = InferOutput<typeof ZNicoScriptReverseTarget>;
type NicoScriptReplace = {
  start: number;
  long: number | undefined;
  keyword: string;
  replace: string;
  range: NicoScriptReplaceRange;
  target: NicoScriptReplaceTarget;
  condition: NicoScriptReplaceCondition;
  color: string | undefined;
  size: CommentSize | undefined;
  font: CommentFont | undefined;
  loc: CommentLoc | undefined;
  no: number;
};
declare const ZNicoScriptReplaceRange: UnionSchema<[LiteralSchema<"単", undefined>, LiteralSchema<"全", undefined>], undefined>;
type NicoScriptReplaceRange = InferOutput<typeof ZNicoScriptReplaceRange>;
declare const ZNicoScriptReplaceTarget: UnionSchema<[LiteralSchema<"コメ", undefined>, LiteralSchema<"投コメ", undefined>, LiteralSchema<"全", undefined>, LiteralSchema<"含まない", undefined>, LiteralSchema<"含む", undefined>], undefined>;
type NicoScriptReplaceTarget = InferOutput<typeof ZNicoScriptReplaceTarget>;
declare const ZNicoScriptReplaceCondition: UnionSchema<[LiteralSchema<"部分一致", undefined>, LiteralSchema<"完全一致", undefined>], undefined>;
type NicoScriptReplaceCondition = InferOutput<typeof ZNicoScriptReplaceCondition>;
type NicoScriptBan = {
  start: number;
  end: number;
};
type NicoScriptDefault = {
  start: number;
  long: number | undefined;
  color: string | undefined;
  size: CommentSize | undefined;
  font: CommentFont | undefined;
  loc: CommentLoc | undefined;
};
type MeasureTextResult = {
  width: number;
  height: number;
  resized: boolean;
  resizedX: boolean;
  resizedY: boolean;
  fontSize: number;
  lineHeight: number;
  content: CommentMeasuredContentItem[];
  charSize: number;
  scaleX: number;
  scale: number;
};
type ButtonParams = {
  message: {
    before: string;
    body: string;
    after: string;
  };
  commentMessage: string;
  commentMail: string[];
  commentVisible: boolean;
  limit: number;
  local: boolean;
  hidden: boolean;
};
type ParsedCommand = {
  loc: CommentLoc | undefined;
  size: CommentSize | undefined;
  fontSize: number | undefined;
  color: string | undefined;
  strokeColor?: string;
  wakuColor?: string;
  fillColor?: string;
  opacity?: number;
  commandScale?: number;
  ignoreScale?: boolean;
  font: CommentFont | undefined;
  full: boolean;
  ender: boolean;
  _live: boolean;
  invisible: boolean;
  long: number | undefined;
  button?: ButtonParams;
};
type MeasureTextInput = FormattedCommentWithFont & {
  resized?: boolean;
  resizedY?: boolean;
  resizedX?: boolean;
  lineHeight?: number;
  charSize?: number;
  scale: number;
  layerScale: number;
};
declare const ZMeasureInput: ObjectSchema<{
  readonly font: UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"mincho", undefined>, LiteralSchema<"gothic", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>;
  readonly content: ArraySchema<UnionSchema<[ObjectSchema<{
    readonly type: LiteralSchema<"spacer", undefined>;
    readonly char: StringSchema<undefined>;
    readonly charWidth: NumberSchema<undefined>;
    readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
    readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
    readonly count: NumberSchema<undefined>;
  }, undefined>, ObjectSchema<{
    readonly type: LiteralSchema<"text", undefined>;
    readonly content: StringSchema<undefined>;
    readonly slicedContent: ArraySchema<StringSchema<undefined>, undefined>;
    readonly isButton: OptionalSchema<BooleanSchema<undefined>, undefined>;
    readonly font: OptionalSchema<UnionSchema<[LiteralSchema<"defont", undefined>, LiteralSchema<"gulim", undefined>, LiteralSchema<"simsun", undefined>], undefined>, undefined>;
    readonly width: OptionalSchema<ArraySchema<NumberSchema<undefined>, undefined>, undefined>;
  }, undefined>], undefined>, undefined>;
  readonly lineHeight: NumberSchema<undefined>;
  readonly charSize: NumberSchema<undefined>;
  readonly lineCount: NumberSchema<undefined>;
}, undefined>;
type MeasureInput = InferOutput<typeof ZMeasureInput>;
type ValueOf<T> = T[keyof T];
declare namespace colors_d_exports {
  export { colors };
}
declare const colors: {
  white: string;
  red: string;
  pink: string;
  orange: string;
  yellow: string;
  green: string;
  cyan: string;
  blue: string;
  purple: string;
  black: string;
  white2: string;
  niconicowhite: string;
  red2: string;
  truered: string;
  pink2: string;
  orange2: string;
  passionorange: string;
  yellow2: string;
  madyellow: string;
  green2: string;
  elementalgreen: string;
  cyan2: string;
  blue2: string;
  marinblue: string;
  purple2: string;
  nobleviolet: string;
  black2: string;
};
declare namespace config_d_exports {
  export { defaultConfig, defaultOptions, resetOptions, setConfig, setOptions, updateConfig };
}
declare let defaultConfig: BaseConfig;
declare const updateConfig: (config: BaseConfig) => void;
declare const setConfig: (config: BaseConfig) => void;
declare const defaultOptions: BaseOptions;
declare const setOptions: (options: BaseOptions) => void;
declare const resetOptions: () => void;
declare namespace fonts_d_exports {
  export { fontTemplates, fonts };
}
declare const fontTemplates: {
  arial: {
    font: string;
    offset: number;
    weight: number;
  };
  gothic: {
    font: string;
    offset: number;
    weight: number;
  };
  gulim: {
    font: string;
    offset: number;
    weight: number;
  };
  mincho: {
    font: string;
    offset: number;
    weight: number;
  };
  simsun: {
    font: string;
    offset: number;
    weight: number;
  };
  macGothicPro6: {
    font: string;
    offset: number;
    weight: number;
  };
  macGothicPro3: {
    font: string;
    offset: number;
    weight: number;
  };
  macMincho: {
    font: string;
    offset: number;
    weight: number;
  };
  macGothic1: {
    font: string;
    offset: number;
    weight: number;
  };
  macGothic2: {
    font: string;
    offset: number;
    weight: number;
  };
  sansSerif600: {
    font: string;
    offset: number;
    weight: number;
  };
  sansSerif400: {
    font: string;
    offset: number;
    weight: number;
  };
  serif: {
    font: string;
    offset: number;
    weight: number;
  };
};
declare const fonts: {
  win7: {
    defont: FontItem;
    gothic: FontItem;
    mincho: FontItem;
  };
  win8_1: {
    defont: FontItem;
    gothic: FontItem;
    mincho: FontItem;
  };
  win: {
    defont: FontItem;
    gothic: FontItem;
    mincho: FontItem;
  };
  mac10_9: {
    defont: FontItem;
    gothic: FontItem;
    mincho: FontItem;
  };
  mac10_11: {
    defont: FontItem;
    gothic: FontItem;
    mincho: FontItem;
  };
  mac: {
    defont: FontItem;
    gothic: FontItem;
    mincho: FontItem;
  };
  other: {
    defont: FontItem;
    gothic: FontItem;
    mincho: FontItem;
  };
};
declare namespace initConfig_d_exports {
  export { initConfig };
}
declare const initConfig: () => void;
//#endregion
//#region src/errors/CanvasRenderingContext2DError.d.ts
declare class CanvasRenderingContext2DError extends Error {
  constructor(options?: {
    [key: string]: unknown;
  });
}
//#endregion
//#region src/errors/InvalidFormatError.d.ts
declare class InvalidFormatError extends Error {
  constructor(options?: {
    [key: string]: unknown;
  });
}
//#endregion
//#region src/errors/InvalidOptionError.d.ts
declare class InvalidOptionError extends Error {
  constructor(options?: {
    [key: string]: unknown;
  });
}
//#endregion
//#region src/errors/NotImplementedError.d.ts
declare class NotImplementedError extends Error {
  pluginName: string;
  methodName: string;
  constructor(pluginName: string, methodName: string, options?: {
    [key: string]: unknown;
  });
}
declare namespace index_d_exports$2 {
  export { CanvasRenderingContext2DError, InvalidFormatError, InvalidOptionError, NotImplementedError };
}
declare namespace eventHandler_d_exports {
  export { EventHandler };
}
declare class EventHandler {
  private handlerList;
  private handlerCounts;
  private readonly banActiveRangeScans;
  private readonly seekDisableActiveRangeScans;
  private readonly jumpActiveRangeScans;
  register<K extends keyof CommentEventHandlerMap>(eventName: K, handler: CommentEventHandlerMap[K]): void;
  remove<K extends keyof CommentEventHandlerMap>(eventName: K, handler: CommentEventHandlerMap[K]): void;
  trigger(vpos: number, lastVpos: number, nicoScripts: NicoScript): void;
  private _updateCounts;
  private _processCommentDisable;
  private _processSeekDisable;
  private _processJump;
  private _execute;
}
declare namespace inputParser_d_exports {
  export { convert2formattedComment as default };
}
declare const convert2formattedComment: (data: unknown, type: InputFormatType) => FormattedComment[];
//#endregion
//#region src/renderer/canvas.d.ts
declare class CanvasRenderer implements IRenderer {
  readonly rendererName = "CanvasRenderer";
  readonly canvas: HTMLCanvasElement;
  readonly video?: HTMLVideoElement;
  private readonly context;
  private padding;
  private width;
  private height;
  private static readonly _MT_CACHE_MAX_SIZE;
  private static _mtCache;
  private static _dsCanvas;
  private static _dsCtx;
  private static _dsScale;
  private static _dsFont;
  private readonly pooled;
  private readonly _onDestroy?;
  private _destroyed;
  constructor(canvas?: HTMLCanvasElement, video?: HTMLVideoElement, padding?: number, onDestroy?: () => void);
  private resetContextState;
  drawVideo(enableLegacyPip: boolean): void;
  getFont(): string;
  getFillStyle(): string | CanvasGradient | CanvasPattern;
  setScale(scale: number, arg1?: number): void;
  drawImage(image: IRenderer, x: number, y: number, width?: number, height?: number): void;
  fillRect(x: number, y: number, width: number, height: number): void;
  strokeRect(x: number, y: number, width: number, height: number): void;
  fillText(text: string, x: number, y: number): void;
  strokeText(text: string, x: number, y: number): void;
  quadraticCurveTo(cpx: number, cpy: number, x: number, y: number): void;
  clearRect(x: number, y: number, width: number, height: number): void;
  clear(): void;
  setFont(font: string): void;
  setFillStyle(color: string): void;
  setStrokeStyle(color: string): void;
  setLineWidth(width: number): void;
  setGlobalAlpha(alpha: number): void;
  setSize(width: number, height: number): void;
  getSize(): {
    width: number;
    height: number;
  };
  getImagePadding(): number;
  measureText(text: string): TextMetrics;
  measureTextAtDrawScale(text: string, drawScale: number): TextMetrics;
  private _measureAtScale;
  beginPath(): void;
  closePath(): void;
  moveTo(x: number, y: number): void;
  lineTo(x: number, y: number): void;
  stroke(): void;
  save(): void;
  restore(): void;
  getCanvas(padding?: number): IRenderer;
  flush(): void;
  invalidateImage(_image: IRenderer): void;
  destroy(): void;
}
//#endregion
//#region src/renderer/html5css.d.ts
declare class HTML5CSSRenderer implements IRenderer {
  readonly rendererName = "HTML5CSSRenderer";
  readonly canvas: HTMLCanvasElement;
  readonly root: HTMLElement;
  readonly layer: HTMLDivElement;
  readonly video?: HTMLVideoElement;
  private helper;
  private helperDirty;
  private helperCursor;
  private pathActive;
  private textDrawnBeforeDom;
  private readonly helperSurfaces;
  private videoSurface?;
  private width;
  private height;
  private state;
  private readonly stateStack;
  private readonly nodes;
  private prevNodeCursor;
  private activeCanvasSet;
  private prevCanvasSet;
  private readonly setupCanvases;
  private destroyed;
  private readonly cloneMap;
  private readonly cloneSourceMap;
  private duplicateCloneBudgetMap;
  private externalCanvasCopyFrameBudget;
  private readonly ownedCanvases;
  private readonly resizeObserver?;
  private readonly originalRootStyle;
  private nodeCursor;
  constructor(root: HTMLElement, video?: HTMLVideoElement);
  destroy(): void;
  drawVideo(enableLegacyPip: boolean): void;
  getFont(): string;
  getFillStyle(): string;
  setScale(scale: number, arg1?: number): void;
  fillRect(x: number, y: number, width: number, height: number): void;
  strokeRect(x: number, y: number, width: number, height: number): void;
  fillText(text: string, x: number, y: number): void;
  strokeText(text: string, x: number, y: number): void;
  quadraticCurveTo(cpx: number, cpy: number, x: number, y: number): void;
  clearRect(_x: number, _y: number, _width: number, _height: number): void;
  setFont(font: string): void;
  setFillStyle(color: string): void;
  setStrokeStyle(color: string): void;
  setLineWidth(width: number): void;
  setGlobalAlpha(alpha: number): void;
  setSize(width: number, height: number): void;
  getSize(): {
    width: number;
    height: number;
  };
  measureText(text: string): TextMetrics;
  beginPath(): void;
  closePath(): void;
  moveTo(x: number, y: number): void;
  lineTo(x: number, y: number): void;
  stroke(): void;
  save(): void;
  restore(): void;
  getCanvas(padding?: number): IRenderer;
  drawImage(image: IRenderer, x: number, y: number, width?: number, height?: number): void;
  flush(): void;
  invalidateImage(image: IRenderer): void;
  private getNode;
  private positionNode;
  private updateObjectFitContain;
  private updateObjectFitContainWithSize;
  private getHelperSurface;
  private prepareHelperSurface;
  private commitHelperSurface;
  private keepVideoSurfaceFirst;
  private setupVideoCanvas;
  private setupSurfaceCanvas;
  private applyHelperScale;
  private recreateCurrentHelperSurface;
  private teardownSurfaceCanvas;
  private resetHelperContextDefaults;
  private shouldDrawOnOverflowHelper;
  private getCanvasByteSize;
  private canReserveCanvasCopyBudget;
  private incrementCanvasCopyBudget;
  private decrementCanvasCopyBudget;
  private reserveDuplicateClone;
  private reserveExternalCanvasCopy;
  private releaseDuplicateClone;
  private releaseExternalCanvasCopy;
  private resetDuplicateCloneAccounting;
  private trimHelperSurfaces;
  private resetState;
  private restoreFrameStartState;
  private toRenderState;
  private normalizeSize;
  private hideNode;
  private getInitialSize;
  private getPositiveNumber;
  private getNumber;
}
//#endregion
//#region src/renderer/webgl2.d.ts
declare class WebGL2Renderer implements IRenderer {
  readonly rendererName = "WebGL2Renderer";
  readonly canvas: HTMLCanvasElement;
  readonly video?: HTMLVideoElement;
  private readonly gl;
  private spriteProg;
  private spriteLocRect;
  private spriteLocProj;
  private spriteLocAlpha;
  private rectProg;
  private rectLocRect;
  private rectLocProj;
  private rectLocColor;
  private quadVAO;
  private quadBuf;
  private readonly maxTextureSize;
  private tileCanvas;
  private tileCtx;
  private readonly proj;
  private scaleX;
  private scaleY;
  private state;
  private readonly stateStack;
  private readonly cmds;
  private readonly texMap;
  private frameCount;
  private helper;
  private helperDirty;
  private redrawNeeded;
  private readonly colorCtx;
  private readonly colorCache;
  private width;
  private height;
  private readonly _onContextLost;
  private readonly _onContextRestored;
  constructor(canvas: HTMLCanvasElement, video?: HTMLVideoElement);
  private _getUniformLocation;
  private _createHelper;
  private _createShader;
  private _createProgram;
  private _initGLResources;
  private _updateProjection;
  private _parseColor;
  private _createTexture;
  private _extractTile;
  private _buildTiles;
  private _uploadTexture;
  private _deleteTiles;
  private _gcTextures;
  private _rebuildGLResources;
  save(): void;
  restore(): void;
  setScale(scale: number, arg1?: number): void;
  getFont(): string;
  setFont(font: string): void;
  getFillStyle(): string;
  setFillStyle(color: string): void;
  setStrokeStyle(color: string): void;
  setLineWidth(width: number): void;
  setGlobalAlpha(alpha: number): void;
  setSize(width: number, height: number): void;
  getSize(): {
    width: number;
    height: number;
  };
  getCanvas(padding?: number): IRenderer;
  clearRect(x: number, y: number, w: number, h: number): void;
  drawImage(image: IRenderer, x: number, y: number, width?: number, height?: number): void;
  fillRect(x: number, y: number, w: number, h: number): void;
  strokeRect(x: number, y: number, w: number, h: number): void;
  drawVideo(enableLegacyPip: boolean): void;
  fillText(text: string, x: number, y: number): void;
  strokeText(text: string, x: number, y: number): void;
  measureText(text: string): TextMetrics;
  beginPath(): void;
  closePath(): void;
  moveTo(x: number, y: number): void;
  lineTo(x: number, y: number): void;
  quadraticCurveTo(cpx: number, cpy: number, x: number, y: number): void;
  stroke(): void;
  flush(): void;
  invalidateImage(image: IRenderer): void;
  needsRedraw(): boolean;
  destroy(): void;
}
declare namespace index_d_exports$1 {
  export { CanvasRenderer, HTML5CSSRenderer, WebGL2Renderer, createRenderer };
}
declare function createRenderer(canvas: HTMLCanvasElement, video?: HTMLVideoElement): IRenderer;
declare namespace typeGuard_d_exports {
  export { MAX_OPTION_SCALE, typeGuard as default };
}
declare const MAX_OPTION_SCALE = 8;
declare const typeGuard: {
  formatted: {
    comment: (i: unknown) => i is FormattedComment;
    comments: (i: unknown) => i is FormattedComment[];
    legacyComment: (i: unknown) => i is FormattedLegacyComment;
    legacyComments: (i: unknown) => i is FormattedLegacyComment[];
  };
  legacy: {
    rawApiResponses: (i: unknown) => i is RawApiResponse[];
    apiChat: (i: unknown) => i is ApiChat;
    apiGlobalNumRes: (i: unknown) => i is ApiGlobalNumRes;
    apiLeaf: (i: unknown) => i is ApiLeaf;
    apiPing: (i: unknown) => i is ApiPing;
    apiThread: (i: unknown) => i is ApiThread;
  };
  xmlDocument: (i: unknown) => i is XMLDocument;
  xml2js: {
    packet: (i: unknown) => i is Xml2jsPacket;
    chat: (i: unknown) => i is Xml2jsChat;
    chatItem: (i: unknown) => i is Xml2jsChatItem;
  };
  legacyOwner: {
    comments: (i: unknown) => i is string;
  };
  owner: {
    comment: (i: unknown) => i is OwnerComment;
    comments: (i: unknown) => i is OwnerComment[];
  };
  v1: {
    comment: (i: unknown) => i is V1Comment;
    comments: (i: unknown) => i is V1Comment[];
    thread: (i: unknown) => i is V1Thread;
    threads: (i: unknown) => i is V1Thread[];
  };
  nicoScript: {
    range: {
      target: (i: unknown) => i is NicoScriptReverseTarget;
    };
    replace: {
      range: (i: unknown) => i is NicoScriptReplaceRange;
      target: (i: unknown) => i is NicoScriptReplaceTarget;
      condition: (i: unknown) => i is NicoScriptReplaceCondition;
    };
  };
  comment: {
    font: (i: unknown) => i is CommentFont;
    loc: (i: unknown) => i is CommentLoc;
    size: (i: unknown) => i is CommentSize;
    command: {
      key: (i: unknown) => i is "full" | "ender" | "_live" | "invisible";
    };
    color: (i: unknown) => i is keyof typeof colors;
    colorCode: (i: unknown) => i is string;
    colorCodeAllowAlpha: (i: unknown) => i is string;
  };
  config: {
    initOptions: (item: unknown) => item is Options;
  };
  internal: {
    CommentMeasuredContentItem: (i: unknown) => i is CommentMeasuredContentItem;
    CommentMeasuredContentItemArray: (i: unknown) => i is CommentMeasuredContentItem[];
    MultiConfigItem: <T>(i: unknown) => i is MultiConfigItem<T>;
    HTML5Fonts: (i: unknown) => i is HTML5Fonts;
    MeasureInput: (i: unknown) => i is MeasureInput;
  };
};
//#endregion
//#region src/utils/array.d.ts
declare const arrayPush: (_array: {
  [key: number]: IComment[];
}, key: number, push: IComment) => void;
declare const arrayEqual: (a: readonly unknown[], b: readonly unknown[]) => boolean;
//#endregion
//#region src/utils/color.d.ts
declare const hex2rgb: (_hex: string) => number[];
declare const hex2rgba: (_hex: string) => number[];
declare const getStrokeColor: (comment: FormattedCommentWithSize, config: BaseConfig) => string;
//#endregion
//#region src/utils/comment.d.ts
declare const DEFAULT_COMMENT_LONG = 300;
declare const DEFAULT_NICOSCRIPT_LONG: number;
declare const MAX_COMMENT_LONG: number;
declare const MAX_NICOSCRIPT_LONG: number;
declare const MAX_AT_BUTTON_COMMAND_CHARS = 16384;
declare const MAX_AT_BUTTON_TEXT_CHARS = 4096;
declare const MAX_AT_BUTTON_MAIL_ENTRIES = 16;
declare const MAX_AT_BUTTON_MAIL_CHARS = 64;
declare const MAX_AT_BUTTON_LIMIT = 100;
declare const MAX_PARSED_COMMAND_MAIL_ENTRIES = 64;
declare const MAX_PARSED_COMMAND_MAIL_CHARS = 128;
declare const MAX_NICOSCRIPT_COMMAND_CHARS = 16384;
declare const MAX_NICOSCRIPT_TEXT_CHARS = 4096;
declare const MAX_LAZY_COMMENT_LOOKAHEAD: number;
declare const getLazyCommentLookahead: (canvasWidth: number) => number;
declare const isLineBreakResize: (comment: MeasureTextInput, config: BaseConfig) => boolean;
declare const getDefaultCommand: (vpos: number, nicoScripts: NicoScript) => DefaultCommand;
declare const parseCommandAndNicoScript: (comment: FormattedComment, ctx: CommentInstanceContext) => ParseCommandAndNicoScriptResult;
declare const isFlashComment: (comment: FormattedComment, config: BaseConfig, options: BaseOptions) => boolean;
declare const isReverseActive: (vpos: number, isOwner: boolean, nicoScripts: NicoScript, rangeCache: RangeCacheContext) => boolean;
declare const isBanActive: (vpos: number, nicoScripts: NicoScript, rangeCache: RangeCacheContext) => boolean;
declare const processFixedComment: (comment: IComment, collision: CollisionItem, timeline: Timeline, lazy: boolean | undefined, config: BaseConfig, touchedTimeline?: Set<number>) => void;
declare const processMovableComment: (comment: IComment, collision: Collision, timeline: Timeline, lazy: boolean | undefined, config: BaseConfig, touchedTimeline?: Set<number>) => void;
declare const getFixedPosY: (comment: IComment, collision: CollisionItem, config: BaseConfig) => number;
declare const getMovablePosY: (comment: IComment, collision: Collision, beforeVpos: number, config: BaseConfig, speed?: number) => number;
declare const getPosY: (_currentPos: number, targetComment: IComment, collision: IComment[] | undefined, config: BaseConfig) => {
  currentPos: number;
  isChanged: boolean;
  isBreak: boolean;
};
declare const getPosX: (comment: FormattedCommentWithSize, vpos: number, config: BaseConfig, isReverse?: boolean) => number;
declare const parseFont: (font: CommentFont, size: string | number, config: BaseConfig) => string;
//#endregion
//#region src/utils/commentArt.d.ts
declare const changeCALayer: (rawData: FormattedComment[], config: BaseConfig) => FormattedComment[];
//#endregion
//#region src/utils/config.d.ts
declare const getConfig: <T>(input: ConfigItem<T>, isFlash?: boolean) => T;
//#endregion
//#region src/utils/flash.d.ts
declare const MAX_FLASH_COMMENT_CHARS = 16384;
declare const MAX_FLASH_COMMENT_LINES = 256;
declare const MAX_FLASH_CONTENT_ITEMS = 2048;
declare const clampFlashContent: (input: string) => {
  content: string;
  lineCount: number;
};
declare const getFlashFontIndex: (part: string, config: BaseConfig) => CommentContentIndex[];
declare const getFlashFontName: (font: CommentFlashFontParsed) => CommentFlashFont;
declare const parseContent: (content: string, config: BaseConfig) => ({
  type: "spacer";
  char: string;
  charWidth: number;
  isButton?: boolean | undefined;
  font?: "defont" | "gulim" | "simsun" | undefined;
  count: number;
} | {
  type: "text";
  content: string;
  slicedContent: string[];
  isButton?: boolean | undefined;
  font?: "defont" | "gulim" | "simsun" | undefined;
  width?: number[] | undefined;
})[];
declare const getButtonParts: (comment: FormattedCommentWithSize, config: BaseConfig) => FormattedCommentWithSize;
declare const buildAtButtonComment: (comment: FormattedCommentWithSize, vpos: number) => FormattedComment | undefined;
//#endregion
//#region src/utils/niconico.d.ts
declare const getLineHeight: (fontSize: CommentSize, isFlash: boolean, config: BaseConfig, resized?: boolean) => number;
declare const getCharSize: (fontSize: CommentSize, isFlash: boolean, config: BaseConfig) => number;
declare const measure: (comment: MeasureInput, renderer: IRenderer, config: BaseConfig, layerScale?: number) => {
  height: number;
  width: number;
  lineWidth: number[];
  itemWidth: number[][];
};
declare const addHTML5PartToResult: (lineContent: CommentContentItem[], part: string, config: BaseConfig, _font?: CommentHTML5Font) => void;
declare const getFontSizeAndScale: (_charSize: number, config: BaseConfig) => {
  scale: number;
  fontSize: number;
};
//#endregion
//#region src/utils/sort.d.ts
declare const nativeSort: <T>(getter: (input: T) => number) => (a: T, b: T) => 1 | 0 | -1;
declare namespace index_d_exports {
  export { DEFAULT_COMMENT_LONG, DEFAULT_NICOSCRIPT_LONG, MAX_AT_BUTTON_COMMAND_CHARS, MAX_AT_BUTTON_LIMIT, MAX_AT_BUTTON_MAIL_CHARS, MAX_AT_BUTTON_MAIL_ENTRIES, MAX_AT_BUTTON_TEXT_CHARS, MAX_COMMENT_LONG, MAX_FLASH_COMMENT_CHARS, MAX_FLASH_COMMENT_LINES, MAX_FLASH_CONTENT_ITEMS, MAX_LAZY_COMMENT_LOOKAHEAD, MAX_NICOSCRIPT_COMMAND_CHARS, MAX_NICOSCRIPT_LONG, MAX_NICOSCRIPT_TEXT_CHARS, MAX_PARSED_COMMAND_MAIL_CHARS, MAX_PARSED_COMMAND_MAIL_ENTRIES, RangeCacheContext, addHTML5PartToResult, arrayEqual, arrayPush, buildAtButtonComment, changeCALayer, clampFlashContent, getButtonParts, getCharSize, getConfig, getDefaultCommand, getFixedPosY, getFlashFontIndex, getFlashFontName, getFontSizeAndScale, getLazyCommentLookahead, getLineHeight, getMovablePosY, getPosX, getPosY, getStrokeColor, hex2rgb, hex2rgba, isBanActive, isFlashComment, isLineBreakResize, isReverseActive, measure, nativeSort, parseCommandAndNicoScript, parseContent, parseFont, processFixedComment, processMovableComment };
}
declare namespace internal_d_exports {
  export { index_d_exports$3 as comments, index_d_exports$4 as contexts, definition, index_d_exports$2 as errors, eventHandler_d_exports as eventHandler, inputParser_d_exports as inputParser, index_d_exports$1 as renderer, typeGuard_d_exports as typeGuard, index_d_exports as utils };
}
declare const definition: {
  colors: typeof colors_d_exports;
  config: typeof config_d_exports;
  fonts: typeof fonts_d_exports;
  initConfig: typeof initConfig_d_exports;
};
//#endregion
//#region src/main.d.ts
declare class NiconiComments {
  enableLegacyPiP: boolean;
  showCollision: boolean;
  showFPS: boolean;
  showCommentCount: boolean;
  private lastVpos;
  private lastEventVpos;
  private lastCursor?;
  private lastFrameBanActive;
  private frameDirty;
  private get lastVposInt();
  private _cachedSplit;
  private lazyCommentOrderSortedByVpos;
  private nextUnprocessedCommentIndex;
  private commentArrayIndexMap;
  private processedCommentIndex;
  private comments;
  private destroyed;
  private readonly renderer;
  private readonly collision;
  private readonly timeline;
  private readonly ctx;
  private readonly eventHandler;
  private plugins;
  static typeGuard: {
    formatted: {
      comment: (i: unknown) => i is FormattedComment;
      comments: (i: unknown) => i is FormattedComment[];
      legacyComment: (i: unknown) => i is FormattedLegacyComment;
      legacyComments: (i: unknown) => i is FormattedLegacyComment[];
    };
    legacy: {
      rawApiResponses: (i: unknown) => i is RawApiResponse[];
      apiChat: (i: unknown) => i is ApiChat;
      apiGlobalNumRes: (i: unknown) => i is ApiGlobalNumRes;
      apiLeaf: (i: unknown) => i is ApiLeaf;
      apiPing: (i: unknown) => i is ApiPing;
      apiThread: (i: unknown) => i is ApiThread;
    };
    xmlDocument: (i: unknown) => i is XMLDocument;
    xml2js: {
      packet: (i: unknown) => i is Xml2jsPacket;
      chat: (i: unknown) => i is Xml2jsChat;
      chatItem: (i: unknown) => i is Xml2jsChatItem;
    };
    legacyOwner: {
      comments: (i: unknown) => i is string;
    };
    owner: {
      comment: (i: unknown) => i is OwnerComment;
      comments: (i: unknown) => i is OwnerComment[];
    };
    v1: {
      comment: (i: unknown) => i is V1Comment;
      comments: (i: unknown) => i is V1Comment[];
      thread: (i: unknown) => i is V1Thread;
      threads: (i: unknown) => i is V1Thread[];
    };
    nicoScript: {
      range: {
        target: (i: unknown) => i is NicoScriptReverseTarget;
      };
      replace: {
        range: (i: unknown) => i is NicoScriptReplaceRange;
        target: (i: unknown) => i is NicoScriptReplaceTarget;
        condition: (i: unknown) => i is NicoScriptReplaceCondition;
      };
    };
    comment: {
      font: (i: unknown) => i is CommentFont;
      loc: (i: unknown) => i is CommentLoc;
      size: (i: unknown) => i is CommentSize;
      command: {
        key: (i: unknown) => i is "full" | "ender" | "_live" | "invisible";
      };
      color: (i: unknown) => i is "white" | "red" | "pink" | "orange" | "yellow" | "green" | "cyan" | "blue" | "purple" | "black" | "white2" | "niconicowhite" | "red2" | "truered" | "pink2" | "orange2" | "passionorange" | "yellow2" | "madyellow" | "green2" | "elementalgreen" | "cyan2" | "blue2" | "marinblue" | "purple2" | "nobleviolet" | "black2";
      colorCode: (i: unknown) => i is string;
      colorCodeAllowAlpha: (i: unknown) => i is string;
    };
    config: {
      initOptions: (item: unknown) => item is Options;
    };
    internal: {
      CommentMeasuredContentItem: (i: unknown) => i is CommentMeasuredContentItem;
      CommentMeasuredContentItemArray: (i: unknown) => i is CommentMeasuredContentItem[];
      MultiConfigItem: <T>(i: unknown) => i is MultiConfigItem<T>;
      HTML5Fonts: (i: unknown) => i is HTML5Fonts;
      MeasureInput: (i: unknown) => i is MeasureInput;
    };
  };
  static default: typeof NiconiComments;
  static readonly BAN_FRAME_POSITION_RESOLUTION_BUDGET = 256;
  static FlashComment: {
    condition: (comment: FormattedComment, config: BaseConfig, options: BaseOptions) => boolean;
    class: typeof FlashComment;
  };
  static internal: typeof internal_d_exports;
  constructor(_renderer: IRenderer | HTMLCanvasElement, data: InputFormat, initOptions?: Options, deferBuild?: boolean);
  /** jusplay: lay out the comments of a deferBuild instance in slices; false if destroyed meanwhile. */
  build(budgetMs?: number): Promise<boolean>;
  destroy(): void;
  private _clearTimeline;
  private _clearCollision;
  private _rebuildCommentArrayIndex;
  private _advanceNextUnprocessedCommentIndex;
  private preRendering;
  private getCommentPos;
  private resolveLazyCommentWindow;
  private sortTimelineComment;
  addComments(...rawComments: FormattedCommentInput[]): void;
  drawCanvas(vpos: number, forceRendering?: boolean, cursor?: Position): boolean;
  private _drawVideo;
  private _drawComments;
  private _drawCollision;
  private _drawFPS;
  private _drawCommentCount;
  addEventListener<K extends keyof CommentEventHandlerMap>(eventName: K, handler: CommentEventHandlerMap[K]): void;
  removeEventListener<K extends keyof CommentEventHandlerMap>(eventName: K, handler: CommentEventHandlerMap[K]): void;
  clear(): void;
  click(vpos: number, pos: Position): void;
  private _log;
}
//#endregion
export { type ApiChat, type ApiGlobalNumRes, type ApiLeaf, type ApiPing, type ApiThread, type BaseConfig, type BaseOptions, type ButtonList, type ButtonParams, type ButtonPartLeft, type ButtonPartMiddle, type ButtonPartRight, type Canvas, type Collision, type CollisionItem, type CollisionPos, type CommentContentIndex, type CommentContentItem, type CommentContentItemText, type CommentDisableEvent, type CommentDisableEventHandler, type CommentEnableEvent, type CommentEnableEventHandler, type CommentEventBase, type CommentEventHandler, type CommentEventHandlerMap, type CommentEventMap, type CommentEventName, type CommentFlashFont, type CommentFlashFontParsed, type CommentFont, type CommentHTML5Font, type CommentLoc, type CommentMeasuredContentItem, type CommentSize, type CommentStageSize, type Config, type ConfigItem, type Context2D, type DefaultCommand, type FlashMode, type FlashScriptChar, type FontItem, type FormattedComment, type FormattedCommentInput, type FormattedCommentWithFont, type FormattedCommentWithSize, type FormattedLegacyComment, type FormattedLegacyCommentInput, type FrameActiveState, type HTML5Fonts, type IComment, type IPlugin, type IPluginConstructor, type IPluginList, type IRenderer, type InputFormat, type InputFormatType, type InputParser, type JumpEvent, type JumpEventHandler, type MeasureInput, type MeasureTextInput, type MeasureTextResult, type MultiConfigItem, type NicoScript, type NicoScriptReplace, type NicoScriptReplaceCondition, type NicoScriptReplaceRange, type NicoScriptReplaceTarget, type NicoScriptReverseTarget, type OWNER_DEFAULT_COLLISION_LAYER, type Options, type OwnerComment, type ParseCommandAndNicoScriptResult, type ParseContentResult, type ParsedCommand, type Platform, type PlatformFont, type Position, type RawApiResponse, type SeekDisableEvent, type SeekDisableEventHandler, type SeekEnableEvent, type SeekEnableEventHandler, type Timeline, type V1Comment, type V1Thread, type VIEWER_DEFAULT_COLLISION_LAYER, type ValueOf, type Xml2jsChat, type Xml2jsChatItem, type Xml2jsPacket, type ZApiChat, type ZApiGlobalNumRes, type ZApiLeaf, type ZApiPing, type ZApiThread, type ZCommentContentItem, type ZCommentContentItemSpacer, type ZCommentContentItemText, type ZCommentDate, type ZCommentDateUsec, type ZCommentFlashFont, type ZCommentFont, type ZCommentHTML5Font, type ZCommentId, type ZCommentLayer, type ZCommentLoc, type ZCommentMeasuredContentItem, type ZCommentMeasuredContentItemText, type ZCommentScore, type ZCommentSize, type ZCommentUserId, type ZCommentVpos, type ZFormattedComment, type ZFormattedLegacyComment, type ZHTML5Fonts, type ZInputFormatType, type ZMeasureInput, type ZNicoScriptReplaceCondition, type ZNicoScriptReplaceRange, type ZNicoScriptReplaceTarget, type ZNicoScriptReverseTarget, type ZOwnerComment, type ZRawApiResponse, type ZV1Comment, type ZV1Thread, type ZXml2jsChat, type ZXml2jsChatItem, type ZXml2jsPacket, NiconiComments as default, type formattedComment, type formattedLegacyComment, type getDefaultCollisionLayer, type inputFormat, type inputFormatType, type isFiniteNumberInRange, type ownerComment, type rawApiResponse, type toFiniteNumberInRange, type v1Thread };
