## Basic Usage
The inference of recombination parameters requires two steps:

1. Calculate Correlation Profile

    For whole-genome alignments (multiple gene alignments), use `mcorr-xmfa`:

    ```sh
    mcorr-xmfa <input XMFA file> <output prefix>
    ```
    The XMFA files should contain only *coding* sequences. The description of XMFA file can be found in [http://darlinglab.org/mauve/user-guide/files.html](http://darlinglab.org/mauve/user-guide/files.html).

    For read alignments, use `mcorr-bam`:
    ```sh
    mcorr-bam <GFF3 file> <sorted BAM file> <output prefix>
    ```
    The GFF3 file is used for extracting the coding regions of the sorted BAM file.

    Both programs will produce two files:
    * a .csv file stores the calculated Correlation Profile, which will be used for fitting in the next step;
    * a .json file stores the (intermediate) Correlation Profile for each gene.

2. Fit the Correlation Profile using `FitP.py`, which can be found in `$HOME/go/src/github.com/kussell-lab/mcorr/cmd/fitting/`:

    ```sh
    mcorr-fit <.csv file> <output_prefix>
    ```

    It will produce two files:

    * `<output_prefix>_best_fit.svg` -- the plots of the Correlation Profile, fitting, and residuals;
    * `<output_prefix>_fit_results.csv` -- the table of fitted parameters.
